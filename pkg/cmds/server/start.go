/*
Copyright The Kubeshield Authors.

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
	"fmt"
	"io"
	"net"

	"kubeshield.dev/identity-server/apis/identity/v1alpha1"
	sampleopenapi "kubeshield.dev/identity-server/client/openapi"
	"kubeshield.dev/identity-server/pkg/apiserver"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/clientcmd"
)

const defaultEtcdPathPrefix = "/registry/identity.kubeshield.io"

// IdentityServerOptions contains state for master/api server
type IdentityServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions

	StdOut io.Writer
	StdErr io.Writer
}

// NewIdentityServerOptions returns a new IdentityServerOptions
func NewIdentityServerOptions(out, errOut io.Writer) *IdentityServerOptions {
	o := &IdentityServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(v1alpha1.SchemeGroupVersion),
			genericoptions.NewProcessInfo("identity-server", meta.Namespace()),
		),

		StdOut: out,
		StdErr: errOut,
	}
	o.RecommendedOptions.Etcd = nil
	o.RecommendedOptions.Admission = nil
	return o
}

// Validate validates IdentityServerOptions
func (o IdentityServerOptions) Validate(args []string) error {
	var errors []error
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in fields required to have valid data
func (o *IdentityServerOptions) Complete() error {
	return nil
}

// Config returns config for the api server given IdentityServerOptions
func (o *IdentityServerOptions) Config() (*apiserver.Config, error) {
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

	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(sampleopenapi.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(apiserver.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = "Identity"
	serverConfig.OpenAPIConfig.Info.Version = "0.1"

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   apiserver.ExtraConfig{},
	}
	return config, nil
}

// RunIdentityServer starts a new IdentityServer given IdentityServerOptions
func (o IdentityServerOptions) RunIdentityServer(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}

	server.GenericAPIServer.AddPostStartHookOrDie("start-identity-server-informers", func(context genericapiserver.PostStartHookContext) error {
		config.GenericConfig.SharedInformerFactory.Start(context.StopCh)
		return nil
	})

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
