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
	"time"

	falco "kubeops.dev/falco-ui-server/apis/falco/v1alpha1"
	scannerreports "kubeops.dev/scanner/apis/reports"
	scannerreportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	scannerscheme "kubeops.dev/scanner/client/clientset/versioned/scheme"
	costinstall "kubeops.dev/ui-server/apis/cost/install"
	costapi "kubeops.dev/ui-server/apis/cost/v1alpha1"
	licenseinstall "kubeops.dev/ui-server/apis/offline/install"
	licenseapi "kubeops.dev/ui-server/apis/offline/v1alpha1"
	policyinstall "kubeops.dev/ui-server/apis/policy/install"
	policyapi "kubeops.dev/ui-server/apis/policy/v1alpha1"
	clustermetacontroller "kubeops.dev/ui-server/pkg/controllers/clustermetadata"
	clusterclaimcontroller "kubeops.dev/ui-server/pkg/controllers/feature"
	projectquotacontroller "kubeops.dev/ui-server/pkg/controllers/projectquota"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/metricshandler"
	genericresourcestorage "kubeops.dev/ui-server/pkg/registry/core/genericresource"
	podviewstorage "kubeops.dev/ui-server/pkg/registry/core/podview"
	projecttorage "kubeops.dev/ui-server/pkg/registry/core/project"
	resourcesservicestorage "kubeops.dev/ui-server/pkg/registry/core/resourceservice"
	resourcesummarystorage "kubeops.dev/ui-server/pkg/registry/core/resourcesummary"
	coststorage "kubeops.dev/ui-server/pkg/registry/cost/reports"
	clusteridstorage "kubeops.dev/ui-server/pkg/registry/identity/clusteridentity"
	inboxtokenreqstorage "kubeops.dev/ui-server/pkg/registry/identity/inboxtokenrequest"
	"kubeops.dev/ui-server/pkg/registry/identity/selfsubjectnamespaceaccessreview"
	siteinfostorage "kubeops.dev/ui-server/pkg/registry/identity/siteinfo"
	"kubeops.dev/ui-server/pkg/registry/meta/chartpresetquery"
	clusterprofilestorage "kubeops.dev/ui-server/pkg/registry/meta/clusterprofile"
	clusterstatusstorage "kubeops.dev/ui-server/pkg/registry/meta/clusterstatus"
	"kubeops.dev/ui-server/pkg/registry/meta/gatewayinfo"
	"kubeops.dev/ui-server/pkg/registry/meta/render"
	"kubeops.dev/ui-server/pkg/registry/meta/renderdashboard"
	"kubeops.dev/ui-server/pkg/registry/meta/rendermenu"
	"kubeops.dev/ui-server/pkg/registry/meta/renderrawgraph"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceblockdefinition"
	resourcecalculatorstorage "kubeops.dev/ui-server/pkg/registry/meta/resourcecalculator"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcedescriptor"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceeditor"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcegraph"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcelayout"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcemanifests"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceoutline"
	"kubeops.dev/ui-server/pkg/registry/meta/resourceoutlinefilter"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcequery"
	"kubeops.dev/ui-server/pkg/registry/meta/resourcetabledefinition"
	"kubeops.dev/ui-server/pkg/registry/meta/usermenu"
	"kubeops.dev/ui-server/pkg/registry/meta/vendormenu"
	"kubeops.dev/ui-server/pkg/registry/offline/addofflinelicense"
	"kubeops.dev/ui-server/pkg/registry/offline/offlinelicense"
	policystorage "kubeops.dev/ui-server/pkg/registry/policy/reports"
	imagestorage "kubeops.dev/ui-server/pkg/registry/scanner/image"
	reportstorage "kubeops.dev/ui-server/pkg/registry/scanner/reports"

	fluxsrc "github.com/fluxcd/source-controller/api/v1"
	"github.com/graphql-go/handler"
	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	openvizapi "go.openviz.dev/apimachinery/apis/openviz/v1alpha1"
	crdinstall "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/install"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"kmodules.xyz/authorizer"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/client-go/meta"
	appcatalogapi "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	promclient "kmodules.xyz/monitoring-agent-api/client"
	rscoreinstall "kmodules.xyz/resource-metadata/apis/core/install"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	identityinstall "kmodules.xyz/resource-metadata/apis/identity/install"
	identityapi "kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	mgmtinstall "kmodules.xyz/resource-metadata/apis/management/install"
	rsinstall "kmodules.xyz/resource-metadata/apis/meta/install"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	uiinstall "kmodules.xyz/resource-metadata/apis/ui/install"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	identitylib "kmodules.xyz/resource-metadata/pkg/identity"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	chartsapi "x-helm.dev/apimachinery/apis/charts/v1alpha1"
	xhelmapi "x-helm.dev/apimachinery/apis/drivers/v1alpha1"
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
	policyinstall.Install(Scheme)
	costinstall.Install(Scheme)
	rsinstall.Install(Scheme)
	uiinstall.Install(Scheme)
	rscoreinstall.Install(Scheme)
	mgmtinstall.Install(Scheme)
	crdinstall.Install(Scheme)
	licenseinstall.Install(Scheme)
	utilruntime.Must(scannerscheme.AddToScheme(Scheme))
	utilruntime.Must(chartsapi.AddToScheme(Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))
	utilruntime.Must(appcatalogapi.AddToScheme(Scheme))
	utilruntime.Must(openvizapi.AddToScheme(Scheme))
	utilruntime.Must(xhelmapi.AddToScheme(Scheme))
	utilruntime.Must(fluxsrc.AddToScheme(Scheme))
	utilruntime.Must(gwv1.Install(Scheme))
	utilruntime.Must(monitoringv1.AddToScheme(Scheme))
	utilruntime.Must(falco.AddToScheme(Scheme))
	utilruntime.Must(clusterv1alpha1.Install(Scheme))

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
	PromConfig   promclient.Config

	BaseURL string
	Token   string
	CACert  []byte
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

	return CompletedConfig{&c}
}

// New returns a new instance of UIServer from the given config.
func (c completedConfig) New(ctx context.Context) (*UIServer, error) {
	genericServer, err := c.GenericConfig.New("kube-ui-server", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	log.SetLogger(klog.NewKlogr())
	setupLog := log.Log.WithName("setup")

	cfg := c.ExtraConfig.ClientConfig
	syncPeriod := 1 * time.Hour
	mgr, err := manager.New(cfg, manager.Options{
		Scheme:                 Scheme,
		Metrics:                metricsserver.Options{BindAddress: ""},
		HealthProbeBindAddress: "",
		LeaderElection:         false,
		LeaderElectionID:       "5b87adeb.ui-server.kubeops.dev",
		//ClientDisableCacheFor: []client.Object{
		//	&core.Pod{},
		//},
		NewClient: cu.NewClient,
		Cache: cache.Options{
			SyncPeriod: &syncPeriod, // Default SyncPeriod is 10 Hours
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to start manager, reason: %v", err)
	}
	ctrlClient := mgr.GetClient()
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create discovery client, reason: %v", err)
	}

	cid, err := clustermeta.ClusterUID(mgr.GetAPIReader())
	if err != nil {
		return nil, err
	}

	rbacAuthorizer := authorizer.NewForManagerOrDie(ctx, mgr)

	builder, err := promclient.NewBuilder(mgr, &c.ExtraConfig.PromConfig)
	if err != nil {
		return nil, err
	}
	if err := builder.Setup(); err != nil {
		return nil, err
	}

	bc, err := identitylib.NewClient(c.ExtraConfig.BaseURL, c.ExtraConfig.Token, c.ExtraConfig.CACert, mgr.GetClient())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create b3 api client")
	}

	pqr, err := projectquotacontroller.NewReconciler(mgr.GetClient(), kc).SetupWithManager(mgr)
	if err != nil {
		klog.Error(err, "unable to create controller", "controller", "ProjectQuota")
		os.Exit(1)
	}

	if err := mgr.Add(manager.RunnableFunc(graph.PollNewResourceTypes(cfg, pqr))); err != nil {
		setupLog.Error(err, "unable to set up resource poller")
		os.Exit(1)
	}

	if err := mgr.Add(manager.RunnableFunc(graph.SetupGraphReconciler(mgr))); err != nil {
		setupLog.Error(err, "unable to set up resource reconciler configurator")
		os.Exit(1)
	}

	if c.ExtraConfig.Token != "" {
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			md, err := bc.Identify(cid)
			if err != nil {
				return err
			}
			return clustermeta.UpsertClusterMetadata(mgr.GetClient(), md)
		})); err != nil {
			setupLog.Error(err, fmt.Sprintf("unable to upsert cluster metadata into configmap %s/%s", metav1.NamespacePublic, kmapi.AceInfoConfigMapName))
			os.Exit(1)
		}

		err = clustermetacontroller.NewReconciler(mgr.GetClient(), bc, cid).SetupWithManager(mgr)
		if err != nil {
			klog.Error(err, "unable to create controller", "controller", "ConfigMap")
			os.Exit(1)
		}
	}

	if clustermeta.DetectClusterManager(mgr.GetClient()).ManagedByOCMSpoke() {
		err = clusterclaimcontroller.NewClusterClaimReconciler(mgr.GetClient()).SetupWithManager(mgr)
		if err != nil {
			klog.Error(err, "unable to create controller", "controller", "ClusterClaim")
			os.Exit(1)
		}
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
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(rsapi.SchemeGroupVersion.Group, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[rsapi.ResourceChartPresetQueries] = chartpresetquery.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceClusterStatuses] = clusterstatusstorage.NewStorage(ctrlClient, kc)
		v1alpha1storage[rsapi.ResourceRenderDashboards] = renderdashboard.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceRenderRawGraphs] = renderrawgraph.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceRenders] = render.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[rsapi.ResourceResourceBlockDefinitions] = resourceblockdefinition.NewStorage()
		v1alpha1storage[rsapi.ResourceResourceCalculators] = resourcecalculatorstorage.NewStorage(ctrlClient, cid, rbacAuthorizer)
		v1alpha1storage[rsapi.ResourceResourceDescriptors] = resourcedescriptor.NewStorage()
		v1alpha1storage[rsapi.ResourceResourceGraphs] = resourcegraph.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceResourceLayouts] = resourcelayout.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceResourceOutlines] = resourceoutline.NewStorage()
		v1alpha1storage[rsapi.ResourceResourceManifests] = resourcemanifests.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceResourceOutlineFilters] = resourceoutlinefilter.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceGatewayInfos] = gatewayinfo.NewStorage(ctrlClient)
		v1alpha1storage[uiapi.ResourceClusterProfiles] = clusterprofilestorage.NewStorage(ctrlClient)
		v1alpha1storage[uiapi.ResourceResourceEditors] = resourceeditor.NewStorage(ctrlClient)
		v1alpha1storage[rsapi.ResourceResourceQueries] = resourcequery.NewStorage(ctrlClient, rbacAuthorizer)
		v1alpha1storage[rsapi.ResourceResourceTableDefinitions] = resourcetabledefinition.NewStorage()

		namespace := meta.PodNamespace()
		v1alpha1storage[rsapi.ResourceRenderMenus] = rendermenu.NewStorage(ctrlClient, kc, namespace)
		v1alpha1storage["usermenus"] = usermenu.NewStorage(ctrlClient, kc, namespace)
		v1alpha1storage["usermenus/available"] = usermenu.NewAvailableStorage(ctrlClient, kc, namespace)
		v1alpha1storage[rsapi.ResourceMenus] = vendormenu.NewStorage(ctrlClient, kc)
		v1alpha1storage[rsapi.ResourceMenus+"/available"] = vendormenu.NewAvailableStorage(ctrlClient, kc)

		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(licenseapi.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[licenseapi.ResourceOfflineLicenses] = offlinelicense.NewStorage(ctrlClient)
		v1alpha1storage[licenseapi.ResourceAddOfflineLicenses] = addofflinelicense.NewStorage(ctrlClient, rbacAuthorizer)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(identityapi.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[identityapi.ResourceClusterIdentities] = clusteridstorage.NewStorage(ctrlClient, bc)
		v1alpha1storage[identityapi.ResourceInboxTokenRequests] = inboxtokenreqstorage.NewStorage(ctrlClient, bc)
		v1alpha1storage[identityapi.ResourceSelfSubjectNamespaceAccessReviews] = selfsubjectnamespaceaccessreview.NewStorage(kc, ctrlClient)
		v1alpha1storage[identityapi.ResourceSiteInfos] = siteinfostorage.NewStorage(mgr.GetConfig(), kc, ctrlClient)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(rscoreapi.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[rscoreapi.ResourceGenericResourceServices] = resourcesservicestorage.NewStorage(ctrlClient, kc, cid, rbacAuthorizer)
		v1alpha1storage[rscoreapi.ResourceGenericResources] = genericresourcestorage.NewStorage(ctrlClient, kc, cid, rbacAuthorizer)
		v1alpha1storage[rscoreapi.ResourcePodViews] = podviewstorage.NewStorage(ctrlClient, rbacAuthorizer, builder)
		v1alpha1storage[rscoreapi.ResourceProjects] = projecttorage.NewStorage(ctrlClient)
		v1alpha1storage[rscoreapi.ResourceResourceSummaries] = resourcesummarystorage.NewStorage(ctrlClient, kc, cid, rbacAuthorizer)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(scannerreports.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[scannerreportsapi.ResourceImages] = imagestorage.NewStorage(ctrlClient)
		v1alpha1storage[scannerreportsapi.ResourceCVEReports] = reportstorage.NewStorage(ctrlClient)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(policyapi.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[policyapi.ResourcePolicyReports] = policystorage.NewStorage(ctrlClient)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(costapi.GroupName, Scheme, metav1.ParameterCodec, Codecs)

		v1alpha1storage := map[string]rest.Storage{}
		v1alpha1storage[costapi.ResourceCostReports] = coststorage.NewStorage(ctrlClient)
		apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}
	{
		// Create metrics handler and fill the stores with metrics store
		// containing Help and Type headers of metrics
		m := metricshandler.MetricsHandler{
			Client: mgr.GetClient(),
		}
		m.Install(genericServer.Handler.NonGoRestfulMux)
	}
	if err := mgr.Add(manager.RunnableFunc(metricshandler.StartMetricsCollector(mgr))); err != nil {
		setupLog.Error(err, "unable to start metrics collector")
		os.Exit(1)
	}
	return s, nil
}
