/*
Copyright AppsCode Inc. and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"context"
	"fmt"

	identityinstall "kubeops.dev/ui-server/apis/identity/install"
	identityv1alpha1 "kubeops.dev/ui-server/apis/identity/v1alpha1"
	uiinstall "kubeops.dev/ui-server/apis/ui/install"
	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	whoamistorage "kubeops.dev/ui-server/pkg/registry/identity/whoami"
	genericresourcestorage "kubeops.dev/ui-server/pkg/registry/ui/genericresource"
	podviewstorage "kubeops.dev/ui-server/pkg/registry/ui/podview"
	resourcesummarystorage "kubeops.dev/ui-server/pkg/registry/ui/resourcesummary"
	"kubeops.dev/ui-server/pkg/shared"

	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/authorizer/rbac"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	// Scheme defines methods for serializing and deserializing API objects.
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	identityinstall.Install(Scheme)
	uiinstall.Install(Scheme)
	_ = clientgoscheme.AddToScheme(Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	ClientConfig *restclient.Config
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// UIServer contains state for a Kubernetes cluster master/api server.
type UIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
	Manager          manager.Manager
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{&c}
}

// New returns a new instance of UIServer from the given config.
func (c completedConfig) New(ctx context.Context) (*UIServer, error) {
	genericServer, err := c.GenericConfig.New("kube-ui-server", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	// ctrl.SetLogger(...)
	log.SetLogger(klogr.New())

	mgr, err := manager.New(c.ExtraConfig.ClientConfig, manager.Options{
		Scheme:                 Scheme,
		MetricsBindAddress:     "",
		Port:                   0,
		HealthProbeBindAddress: "",
		LeaderElection:         false,
		LeaderElectionID:       "5b87adeb.ui.kubedb.com",
		ClientDisableCacheFor: []client.Object{
			&core.Namespace{},
			&core.Secret{},
			&core.Pod{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to start manager, reason: %v", err)
	}
	ctrlClient := mgr.GetClient()

	r := reconcile.Func(func(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	})

	if err := builder.ControllerManagedBy(mgr).For(&rbacv1.ClusterRole{}).Complete(r); err != nil {
		return nil, err
	}
	if err := builder.ControllerManagedBy(mgr).For(&rbacv1.ClusterRoleBinding{}).Complete(r); err != nil {
		return nil, err
	}
	if err := builder.ControllerManagedBy(mgr).For(&rbacv1.Role{}).Complete(r); err != nil {
		return nil, err
	}
	if err := builder.ControllerManagedBy(mgr).For(&rbacv1.RoleBinding{}).Complete(r); err != nil {
		return nil, err
	}
	rbacAuthorizer := rbac.NewForManagerOrDie(ctx, mgr)
	ki, err := shared.GetKubernetesInfo(mgr.GetConfig(), kubernetes.NewForConfigOrDie(mgr.GetConfig()))

	s := &UIServer{
		GenericAPIServer: genericServer,
		Manager:          mgr,
	}

	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(identityv1alpha1.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[identityv1alpha1.ResourceWhoAmIs] = whoamistorage.NewStorage()
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(uiv1alpha1.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[uiv1alpha1.ResourcePodViews] = podviewstorage.NewStorage(ctrlClient, rbacAuthorizer, nil)
		v1alpha1storage[uiv1alpha1.ResourceGenericResources] = genericresourcestorage.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[uiv1alpha1.ResourceResourceSummaries] = resourcesummarystorage.NewStorage(ctrlClient, rbacAuthorizer, ki)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
