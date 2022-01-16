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
	"os"

	identityinstall "kubeops.dev/ui-server/apis/identity/install"
	identityv1alpha1 "kubeops.dev/ui-server/apis/identity/v1alpha1"
	uiinstall "kubeops.dev/ui-server/apis/ui/install"
	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/prometheus"
	"kubeops.dev/ui-server/pkg/registry"
	siteinfostorage "kubeops.dev/ui-server/pkg/registry/auditor/siteinfo"
	whoamistorage "kubeops.dev/ui-server/pkg/registry/identity/whoami"
	"kubeops.dev/ui-server/pkg/registry/meta/render"
	"kubeops.dev/ui-server/pkg/registry/meta/renderapi"
	"kubeops.dev/ui-server/pkg/registry/meta/renderpage"
	"kubeops.dev/ui-server/pkg/registry/meta/rendersection"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceblockdefinition"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcedescriptor"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcegraph"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcelayout"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceoutline"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcetabledefinition"
	genericresourcestorage "kubeops.dev/ui-server/pkg/registry/ui/genericresource"
	podviewstorage "kubeops.dev/ui-server/pkg/registry/ui/podview"
	resourcesservicestorage "kubeops.dev/ui-server/pkg/registry/ui/resourceservice"
	resourcesummarystorage "kubeops.dev/ui-server/pkg/registry/ui/resourcesummary"

	"github.com/graphql-go/handler"
	core "k8s.io/api/core/v1"
	crdinstall "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/install"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/authorizer/rbac"
	cu "kmodules.xyz/client-go/client"
	"kmodules.xyz/custom-resources/apis/auditor"
	auditorinstall "kmodules.xyz/custom-resources/apis/auditor/install"
	auditorv1alpha1 "kmodules.xyz/custom-resources/apis/auditor/v1alpha1"
	"kmodules.xyz/resource-metadata/apis/meta"
	metainstall "kmodules.xyz/resource-metadata/apis/meta/install"
	metav1alpha1 "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	// Scheme defines methods for serializing and deserializing API objects.
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	auditorinstall.Install(Scheme)
	identityinstall.Install(Scheme)
	metainstall.Install(Scheme)
	uiinstall.Install(Scheme)
	crdinstall.Install(Scheme)
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))

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
	PromConfig   prometheus.Config
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
	setupLog := log.Log.WithName("setup")

	cfg := c.ExtraConfig.ClientConfig
	mgr, err := manager.New(cfg, manager.Options{
		Scheme:                 Scheme,
		MetricsBindAddress:     "",
		Port:                   0,
		HealthProbeBindAddress: "",
		LeaderElection:         false,
		LeaderElectionID:       "5b87adeb.ui.kubedb.com",
		ClientDisableCacheFor: []client.Object{
			&core.Pod{},
		},
		NewClient: cu.NewClient,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to start manager, reason: %v", err)
	}
	ctrlClient := mgr.GetClient()

	cid, err := cu.ClusterUID(mgr.GetAPIReader())
	if err != nil {
		return nil, err
	}

	pc, err := c.ExtraConfig.PromConfig.NewPrometheusClient()
	if err != nil {
		return nil, err
	}

	rbacAuthorizer := rbac.NewForManagerOrDie(ctx, mgr)

	if err := mgr.Add(manager.RunnableFunc(graph.PollNewResourceTypes(cfg))); err != nil {
		setupLog.Error(err, "unable to set up resource poller")
		os.Exit(1)
	}

	if err := mgr.Add(manager.RunnableFunc(graph.SetupGraphReconciler(mgr))); err != nil {
		setupLog.Error(err, "unable to set up resource reconciler configurator")
		os.Exit(1)
	}

	s := &UIServer{
		GenericAPIServer: genericServer,
		Manager:          mgr,
	}

	{
		h := handler.New(&handler.Config{
			Schema:     &graph.Schema,
			Pretty:     true,
			GraphiQL:   false,
			Playground: true,
		})
		genericServer.Handler.NonGoRestfulMux.Handle("/graphql", h)
		klog.InfoS("GraphQL handler registered!")
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(meta.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		// TODO: remove
		v1alpha1storage[metav1alpha1.ResourceRenderPages] = renderpage.NewStorage(cfg, ctrlClient, rbacAuthorizer)
		v1alpha1storage[metav1alpha1.ResourceRenderSections] = rendersection.NewStorage(cfg, ctrlClient, rbacAuthorizer)

		v1alpha1storage[metav1alpha1.ResourceResourceDescriptors] = resourcedescriptor.NewStorage()
		v1alpha1storage[metav1alpha1.ResourceResourceGraphs] = resourcegraph.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[metav1alpha1.ResourceRenders] = render.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[metav1alpha1.ResourceRenderAPIs] = renderapi.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[metav1alpha1.ResourceResourceBlockDefinitions] = resourceblockdefinition.NewStorage()
		v1alpha1storage[metav1alpha1.ResourceResourceLayouts] = resourcelayout.NewStorage(ctrlClient)
		v1alpha1storage[metav1alpha1.ResourceResourceOutlines] = resourceoutline.NewStorage()
		v1alpha1storage[metav1alpha1.ResourceResourceTableDefinitions] = resourcetabledefinition.NewStorage()

		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(auditor.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[auditorv1alpha1.ResourceSiteInfos] = registry.RESTInPeace(siteinfostorage.NewStorage(mgr.GetConfig(), ctrlClient))
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
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
		v1alpha1storage[uiv1alpha1.ResourcePodViews] = podviewstorage.NewStorage(ctrlClient, rbacAuthorizer, pc)
		v1alpha1storage[uiv1alpha1.ResourceGenericResources] = genericresourcestorage.NewStorage(ctrlClient, cid, rbacAuthorizer)
		v1alpha1storage[uiv1alpha1.ResourceGenericResourceServices] = resourcesservicestorage.NewStorage(ctrlClient, cid, rbacAuthorizer)
		v1alpha1storage[uiv1alpha1.ResourceResourceSummaries] = resourcesummarystorage.NewStorage(ctrlClient, cid, rbacAuthorizer)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
