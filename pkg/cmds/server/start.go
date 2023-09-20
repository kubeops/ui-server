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
	"net/http"
	"strconv"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	costapi "kubeops.dev/ui-server/apis/cost/v1alpha1"
	identityv1alpha1 "kubeops.dev/ui-server/apis/identity/v1alpha1"
	policyapi "kubeops.dev/ui-server/apis/policy/v1alpha1"
	"kubeops.dev/ui-server/pkg/apiserver"
	featurecontroller "kubeops.dev/ui-server/pkg/controllers/feature"
	"kubeops.dev/ui-server/pkg/metricshandler"

	fluxcd "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	v "gomodules.xyz/x/version"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/features"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
	ou "kmodules.xyz/client-go/openapi"
	"kmodules.xyz/client-go/tools/clientcmd"
	auditorv1alpha1 "kmodules.xyz/custom-resources/apis/auditor/v1alpha1"
	promclient "kmodules.xyz/monitoring-agent-api/client"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	ui "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const defaultEtcdPathPrefix = "/registry/k8s.appscode.com"

// UIServerOptions contains state for master/api server
type UIServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	PrometheusOptions  *promclient.Config
	ExtraOptions       *ExtraOptions

	StdOut io.Writer
	StdErr io.Writer
}

// NewUIServerOptions returns a new UIServerOptions
func NewUIServerOptions(out, errOut io.Writer) *UIServerOptions {
	_ = feature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.APIPriorityAndFairness))
	o := &UIServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(
				auditorv1alpha1.SchemeGroupVersion,
				rsapi.SchemeGroupVersion,
				identityv1alpha1.GroupVersion,
				corev1alpha1.SchemeGroupVersion,
			),
		),
		PrometheusOptions: promclient.NewPrometheusConfig(),
		ExtraOptions:      NewExtraOptions(),
		StdOut:            out,
		StdErr:            errOut,
	}
	o.RecommendedOptions.Etcd = nil
	o.RecommendedOptions.Admission = nil
	return o
}

func (o UIServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
	o.PrometheusOptions.AddFlags(fs)
}

// Validate validates UIServerOptions
func (o UIServerOptions) Validate(args []string) error {
	var errors []error
	errors = append(errors, o.RecommendedOptions.Validate()...)
	errors = append(errors, o.PrometheusOptions.Validate())
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
	// Disable installing the default apiserver /metrics handler
	// ref: https://github.com/kubernetes/apiserver/blob/v0.21.0/pkg/server/config.go#L785-L791
	serverConfig.EnableMetrics = false
	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}
	// Fixes https://github.com/Azure/AKS/issues/522
	clientcmd.Fix(serverConfig.ClientConfig)

	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
		ou.GetDefinitions(
			auditorv1alpha1.GetOpenAPIDefinitions,
			identityv1alpha1.GetOpenAPIDefinitions,
			corev1alpha1.GetOpenAPIDefinitions,
		),
		openapi.NewDefinitionNamer(apiserver.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = "kube-ui-server"
	serverConfig.OpenAPIConfig.Info.Version = v.Version.Version
	serverConfig.OpenAPIConfig.IgnorePrefixes = []string{
		"/swaggerapi",
		fmt.Sprintf("/apis/%s/%s", corev1alpha1.GroupName, corev1alpha1.ResourceProjects),

		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceChartPresetQueries),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceDescriptors),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceGraphs),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceRenders),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceQueries),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceRenderRawGraphs),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceRenderDashboards),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceBlockDefinitions),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceBlockDefinitions),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceLayouts),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceOutlines),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceResourceTableDefinitions),

		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceRenderMenus),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, rsapi.ResourceMenus),
		fmt.Sprintf("/apis/%s/%s", rsapi.SchemeGroupVersion, "usermenus"),

		fmt.Sprintf("/apis/%s/%s", reportsapi.SchemeGroupVersion, reportsapi.ResourceImages),
		fmt.Sprintf("/apis/%s/%s", reportsapi.SchemeGroupVersion, reportsapi.ResourceCVEReports),

		fmt.Sprintf("/apis/%s/%s", policyapi.SchemeGroupVersion, policyapi.ResourcePolicyReports),

		fmt.Sprintf("/apis/%s/%s", costapi.SchemeGroupVersion, costapi.ResourceCostReports),

		fmt.Sprintf("/apis/%s/%s", auditorv1alpha1.SchemeGroupVersion, auditorv1alpha1.ResourceSiteInfos),

		fmt.Sprintf("/apis/%s/%s", ui.SchemeGroupVersion, ui.ResourceFeatures),
	}

	extraConfig := apiserver.ExtraConfig{
		ClientConfig: serverConfig.ClientConfig,
		PromConfig:   *o.PrometheusOptions,
	}
	if err := o.ExtraOptions.ApplyTo(&extraConfig); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   extraConfig,
	}
	return config, nil
}

// RunUIServer starts a new UIServer given UIServerOptions
func (o UIServerOptions) RunUIServer(ctx context.Context) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	{
		metricshandler.RegisterSelfMetrics()
		telemetryMux := buildTelemetryServer(legacyregistry.DefaultGatherer)
		telemetryListenAddress := net.JoinHostPort(o.ExtraOptions.TelemetryHost, strconv.Itoa(o.ExtraOptions.TelemetryPort))
		telemetryServer := http.Server{Handler: telemetryMux, Addr: telemetryListenAddress}

		go func() {
			klog.Infof("Starting self metrics server: %s", telemetryListenAddress)
			if err := telemetryServer.ListenAndServe(); err != nil {
				klog.Errorln(err)
			}
		}()
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

	utilruntime.Must(ui.AddToScheme(server.Manager.GetScheme()))
	utilruntime.Must(fluxcd.AddToScheme(server.Manager.GetScheme()))
	fr := &featurecontroller.FeatureReconciler{
		Client:    server.Manager.GetClient(),
		Scheme:    server.Manager.GetScheme(),
		APIReader: server.Manager.GetAPIReader(),
	}
	if err := fr.SetupWithManager(server.Manager); err != nil {
		return err
	}

	setupLog := log.Log.WithName("setup")
	setupLog.Info("starting manager")
	return server.Manager.Start(ctx)
}

func buildTelemetryServer(registry prometheus.Gatherer) *http.ServeMux {
	mux := http.NewServeMux()

	// Add metricsPath
	mux.Handle(metricshandler.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorLog: promLogger{}}))
	// Add index
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
             <head><title>Metrics for kube-ui-server by AppsCode</title></head>
             <body>
             <h1>Metrics</h1>
			 <ul>
             <li><a href='` + metricshandler.MetricsPath + `'>metrics</a></li>
			 </ul>
             </body>
             </html>`))
	})
	return mux
}

// promLogger implements promhttp.Logger
type promLogger struct{}

func (pl promLogger) Println(v ...interface{}) {
	klog.Error(v...)
}

// promLogger implements the Logger interface
func (pl promLogger) Log(v ...interface{}) error {
	klog.Info(v...)
	return nil
}
