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

package server

import (
	"context"
	"fmt"
	"io"
	"net"

	identityv1alpha1 "kubeops.dev/ui-server/apis/identity/v1alpha1"
	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	"kubeops.dev/ui-server/pkg/apiserver"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/features"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/feature"
	common "k8s.io/kube-openapi/pkg/common"
	"kmodules.xyz/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const defaultEtcdPathPrefix = "/registry/k8s.appscode.com"

// UIServerOptions contains state for master/api server
type UIServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions

	StdOut io.Writer
	StdErr io.Writer
}

// NewUIServerOptions returns a new UIServerOptions
func NewUIServerOptions(out, errOut io.Writer) *UIServerOptions {
	_ = feature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.APIPriorityAndFairness))
	o := &UIServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(identityv1alpha1.GroupVersion, uiv1alpha1.GroupVersion),
		),

		StdOut: out,
		StdErr: errOut,
	}
	o.RecommendedOptions.Etcd = nil
	o.RecommendedOptions.Admission = nil
	return o
}

// Validate validates UIServerOptions
func (o UIServerOptions) Validate(args []string) error {
	var errors []error
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in fields required to have valid data
func (o *UIServerOptions) Complete() error {
	return nil
}

// Config returns config for the api server given UIServerOptions
func (o *UIServerOptions) Config() (*apiserver.Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}
	// Fixes https://github.com/Azure/AKS/issues/522
	clientcmd.Fix(serverConfig.ClientConfig)

	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		identitydefs := identityv1alpha1.GetOpenAPIDefinitions(ref)
		uidefs := uiv1alpha1.GetOpenAPIDefinitions(ref)
		defs := make(map[string]common.OpenAPIDefinition, len(identitydefs)+len(uidefs))
		for k, v := range identitydefs {
			defs[k] = v
		}
		for k, v := range uidefs {
			defs[k] = v
		}
		return defs
	}, openapi.NewDefinitionNamer(apiserver.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = "kube-ui-server"
	serverConfig.OpenAPIConfig.Info.Version = "v0.0.1"

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   apiserver.ExtraConfig{ClientConfig: serverConfig.ClientConfig},
	}
	return config, nil
}

// RunUIServer starts a new UIServer given UIServerOptions
func (o UIServerOptions) RunUIServer(ctx context.Context) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New(ctx)
	if err != nil {
		return err
	}

	server.GenericAPIServer.AddPostStartHookOrDie("start-ui-server-informers", func(context genericapiserver.PostStartHookContext) error {
		config.GenericConfig.SharedInformerFactory.Start(context.StopCh)
		return nil
	})

	err = server.Manager.Add(manager.RunnableFunc(func(ctx context.Context) error {
		return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
	}))
	if err != nil {
		return err
	}

	setupLog := log.Log.WithName("setup")
	setupLog.Info("starting manager")
	return server.Manager.Start(ctx)
}
