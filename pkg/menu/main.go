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

//nolint
package menu

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	flag "github.com/spf13/pflag"
	"github.com/unrolled/render"
	"go.wandrs.dev/binding"
	httpw "go.wandrs.dev/http"
	"gomodules.xyz/logs"
	"gomodules.xyz/pointer"
	"gomodules.xyz/x/crypto/rand"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	meta_util "kmodules.xyz/client-go/meta"
	clientcmdutil "kmodules.xyz/client-go/tools/clientcmd"
	"kmodules.xyz/client-go/tools/converter"
	"kmodules.xyz/client-go/tools/parser"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	packv1alpha1 "kubepack.dev/kubepack/apis/kubepack/v1alpha1"
	"kubepack.dev/kubepack/pkg/lib"
	appapi "kubepack.dev/lib-app/api/v1alpha1"
	"kubepack.dev/lib-app/pkg/editor"
	"kubepack.dev/lib-app/pkg/handler"
	"kubepack.dev/lib-helm/pkg/action"
	"kubepack.dev/lib-helm/pkg/getter"
	"kubepack.dev/lib-helm/pkg/repo"
	"kubepack.dev/lib-helm/pkg/values"
	chartsapi "kubepack.dev/preset/apis/charts/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

/*
const (
	MenuAccordion MenuMode = "Accordion"
	MenuDropDown  MenuMode = "DropDown"
	MenuGallery   MenuMode = "Gallery"
)
*/

var (
	masterURL      = ""
	kubeconfigPath = filepath.Join(homedir.HomeDir(), ".kube", "config")

	url     = "https://raw.githubusercontent.com/kubepack/preset-testdata/master/stable/"
	name    = "hello"
	version = "0.1.0"
)

func main() {
	flag.StringVar(&masterURL, "master", masterURL, "The address of the Kubernetes API server (overrides any value in kubeconfig)")
	flag.StringVar(&kubeconfigPath, "kubeconfig", kubeconfigPath, "Path to kubeconfig file with authorization information (the master location is set by the master flag).")
	flag.StringVar(&url, "url", url, "Chart repo url")
	flag.StringVar(&name, "name", name, "Name of bundle")
	flag.StringVar(&version, "version", version, "Version of bundle")
	flag.Parse()

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterURL}})
	cfg, err := cc.ClientConfig()
	if err != nil {
		klog.Fatal(err)
	}

	client := kubernetes.NewForConfigOrDie(cfg)
	kc, err := NewClient(cfg)
	if err != nil {
		klog.Fatal(err)
	}

	menu, err := GenerateCompleteMenu(kc, client.Discovery())
	if err != nil {
		klog.Fatal(err)
	}
	data, _ := yaml.Marshal(menu)
	fmt.Println(string(data))
}

func main__() {
	flag.StringVar(&masterURL, "master", masterURL, "The address of the Kubernetes API server (overrides any value in kubeconfig)")
	flag.StringVar(&kubeconfigPath, "kubeconfig", kubeconfigPath, "Path to kubeconfig file with authorization information (the master location is set by the master flag).")
	flag.StringVar(&url, "url", url, "Chart repo url")
	flag.StringVar(&name, "name", name, "Name of bundle")
	flag.StringVar(&version, "version", version, "Version of bundle")
	flag.Parse()

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterURL}})
	kubeconfig, err := cc.RawConfig()
	if err != nil {
		klog.Fatal(err)
	}
	getter := clientcmdutil.NewClientGetter(&kubeconfig)

	cfg, err := getter.ToRESTConfig()
	if err != nil {
		klog.Fatal(err)
	}
	kc, err := NewClient(cfg)
	if err != nil {
		klog.Fatal(err)
	}

	chrt, err := lib.DefaultRegistry.GetChart(url, name, version)
	if err != nil {
		klog.Fatal(err)
	}

	cpsMap, err := LoadVendorPresets(chrt)
	if err != nil {
		klog.Fatal(err)
	}
	PrintGPS(cpsMap)

	ps := chartsapi.ClusterChartPreset{
		TypeMeta: metav1.TypeMeta{
			APIVersion: chartsapi.GroupVersion.String(),
			Kind:       chartsapi.ResourceKindClusterChartPreset,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "ui-hello",
		},
		Spec: chartsapi.ClusterChartPresetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels:      map[string]string{},
				MatchExpressions: nil,
			},
			UsePresets: []core.TypedLocalObjectReference{
				{
					APIGroup: pointer.StringP(chartsapi.GroupVersion.Group),
					Kind:     chartsapi.ResourceKindChartPreset,
					Name:     "unified",
				},
			},
			Values: nil,
		},
	}
	valOpts, err := LoadClusterChartPresetValues(kc, cpsMap, &ps)
	if err != nil {
		klog.Fatal(err)
	}

	namespace := "default"
	i, err := action.NewInstaller(getter, namespace, "secret")
	if err != nil {
		klog.Fatal(err)
	}
	i.WithRegistry(lib.DefaultRegistry).
		WithOptions(action.InstallOptions{
			ChartURL:     url,
			ChartName:    name,
			Version:      version,
			Values:       valOpts,
			DryRun:       false,
			DisableHooks: false,
			Replace:      false,
			Wait:         false,
			Devel:        false,
			Timeout:      0,
			Namespace:    namespace,
			ReleaseName:  rand.WithUniqSuffix(name),
			Atomic:       false,
			SkipCRDs:     false,
		})
	rel, _, err := i.Run()
	if err != nil {
		klog.Fatal(err)
	}
	fmt.Println(rel)
}

func NewClient(cfg *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = rsapi.AddToScheme(scheme)
	_ = chartsapi.AddToScheme(scheme)

	log.SetLogger(klogr.New())
	cfg.QPS = 100
	cfg.Burst = 100

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
}

func LoadPresetValues(kc client.Client, cpsMap map[string]*chartsapi.VendorChartPreset, in *chartsapi.ClusterChartPreset, ns string) (values.Options, error) {
	sel, err := metav1.LabelSelectorAsSelector(in.Spec.Selector)
	if err != nil {
		return values.Options{}, err
	}

	var opts values.Options
	err = mergeClusterChartPresetValues(kc, cpsMap, sel, in, &opts)
	if err != nil {
		return values.Options{}, err
	}

	var ps chartsapi.ChartPreset
	err = kc.Get(context.TODO(), client.ObjectKey{Namespace: ns, Name: in.Name}, &ps)
	if err == nil {
		opts.ValueBytes = append(opts.ValueBytes, ps.Spec.Values.Raw)
	} else if client.IgnoreNotFound(err) != nil {
		return values.Options{}, err
	}
	return opts, nil
}

func LoadClusterChartPresetValues(kc client.Client, cpsMap map[string]*chartsapi.VendorChartPreset, in *chartsapi.ClusterChartPreset) (values.Options, error) {
	sel, err := metav1.LabelSelectorAsSelector(in.Spec.Selector)
	if err != nil {
		return values.Options{}, err
	}

	var opts values.Options
	err = mergeClusterChartPresetValues(kc, cpsMap, sel, in, &opts)
	if err != nil {
		return values.Options{}, err
	}
	return opts, nil
}

func mergeClusterChartPresetValues(kc client.Client, cpsMap map[string]*chartsapi.VendorChartPreset, sel labels.Selector, in *chartsapi.ClusterChartPreset, opts *values.Options) error {
	for _, ps := range in.Spec.UsePresets {
		if ps.Kind == chartsapi.ResourceKindChartPreset {
			obj, ok := cpsMap[ps.Name]
			if !ok {
				return fmt.Errorf("missing VendorChartPreset %s", ps.Name)
			}
			if sel.Matches(labels.Set(obj.Labels)) {
				if err := mergeChartPresetValues(kc, cpsMap, sel, obj, opts); err != nil {
					return err
				}
			}
		} else if ps.Kind == chartsapi.ResourceKindClusterChartPreset {
			var obj chartsapi.ClusterChartPreset
			err := kc.Get(context.TODO(), client.ObjectKey{Namespace: in.Namespace, Name: ps.Name}, &obj)
			if err != nil {
				return err
			}
			if sel.Matches(labels.Set(obj.Labels)) {
				if err := mergeClusterChartPresetValues(kc, cpsMap, sel, &obj, opts); err != nil {
					return err
				}
			}
		}
	}
	if in.Spec.Values != nil && in.Spec.Values.Raw != nil {
		opts.ValueBytes = append(opts.ValueBytes, in.Spec.Values.Raw)
	}
	return nil
}

func mergeChartPresetValues(kc client.Client, cpsMap map[string]*chartsapi.VendorChartPreset, sel labels.Selector, in *chartsapi.VendorChartPreset, opts *values.Options) error {
	for _, ps := range in.Spec.UsePresets {
		obj, ok := cpsMap[ps.Name]
		if !ok {
			return fmt.Errorf("missing VendorChartPreset %s", ps.Name)
		}
		if !sel.Matches(labels.Set(obj.Labels)) {
			continue
		}
		if err := mergeChartPresetValues(kc, cpsMap, sel, obj, opts); err != nil {
			return err
		}
	}
	if in.Spec.Values != nil && in.Spec.Values.Raw != nil {
		opts.ValueBytes = append(opts.ValueBytes, in.Spec.Values.Raw)
	}
	return nil
}

//func LoadClusterChartPresets(kc client.Client, chrt *repo.ChartExtended) (map[string]*chartsapi.VendorChartPreset, error) {
//	cpsMap, err := LoadVendorPresets(chrt)
//	if err != nil {
//		return nil, err
//	}
//	var gpsList chartsapi.ChartPresetList
//	err = kc.List(context.TODO(), &gpsList)
//	if client.IgnoreNotFound(err) != nil {
//		return nil, err
//	}
//	for _, item := range gpsList.Items {
//		cpsMap[item.Name] = &item
//	}
//	return cpsMap, nil
//}

func LoadVendorPresets(chrt *repo.ChartExtended) (map[string]*chartsapi.VendorChartPreset, error) {
	cpsMap := map[string]*chartsapi.VendorChartPreset{}
	for _, f := range chrt.Raw {
		if !strings.HasPrefix(f.Name, "presets/") {
			continue
		}
		if err := parser.ProcessResources(f.Data, func(ri parser.ResourceInfo) error {
			if ri.Object.GroupVersionKind() != chartsapi.GroupVersion.WithKind(chartsapi.ResourceKindVendorChartPreset) {
				return nil
			}

			var obj chartsapi.VendorChartPreset
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ri.Object.UnstructuredContent(), &obj); err != nil {
				return errors.Wrapf(err, "failed to convert from unstructured obj %q in file %s", ri.Object.GetName(), ri.Filename)
			}
			cpsMap[obj.Name] = &obj

			return nil
		}); err != nil {
			return nil, err
		}

	}
	return cpsMap, nil
}

func PrintGPS(cpsMap map[string]*chartsapi.VendorChartPreset) {
	names := make([]string, 0, len(cpsMap))
	for k := range cpsMap {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}
}

func main_() {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true)
	kubeConfigFlags.AddFlags(pflag.CommandLine)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(pflag.CommandLine)

	logs.Init(nil, true)
	defer logs.FlushLogs()

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	m := chi.NewRouter()
	m.Use(middleware.RequestID)
	m.Use(middleware.RealIP)
	m.Use(middleware.Logger) // middlewares.NewLogger()
	m.Use(middleware.Recoverer)
	m.Use(binding.Injector(render.New()))

	// PUBLIC
	m.Route("/bundleview", func(m chi.Router) {
		m.With(binding.JSON(packv1alpha1.ChartRepoRef{})).Get("/", binding.HandlerFunc(GetBundleViewForChart))

		// Generate Order for a BundleView
		m.With(binding.JSON(packv1alpha1.BundleView{})).Post("/orders", binding.HandlerFunc(CreateOrderForBundle))
	})

	// PUBLIC
	m.Route("/packageview", func(m chi.Router) {
		m.With(binding.JSON(packv1alpha1.ChartRepoRef{})).Get("/", binding.HandlerFunc(GetPackageViewForChart))

		// PUBLIC
		m.With(binding.JSON(packv1alpha1.ChartRepoRef{})).Get("/files", binding.HandlerFunc(ListPackageFiles))

		// PUBLIC
		m.With(binding.JSON(packv1alpha1.ChartRepoRef{})).Get("/files/*", binding.HandlerFunc(GetPackageFile))

		// Generate Order for a Editor PackageView / Chart
		m.With(binding.JSON(appapi.ChartOrder{})).Post("/orders", binding.HandlerFunc(CreateOrderForPackage))
	})

	m.Route("/editor", func(m chi.Router) {
		m.Use(binding.JSON(map[string]interface{}{}))

		// PUBLIC
		// INITIAL Model (Values)
		// GET vs POST (Get makes more sense, but do we send so much data via query string?)
		// With POST, we can send large payloads without any non-standard limits
		// https://stackoverflow.com/a/812962
		m.Put("/model", binding.HandlerFunc(GenerateEditorModelFromOptions))
		m.Put("/manifest", binding.HandlerFunc(PreviewEditorManifest))
		m.Put("/resources", binding.HandlerFunc(PreviewEditorResources))
	})

	// PUBLIC
	/*
		m.Get("/products", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products

			phase := ctx.R().Params("phase")
			var out packv1alpha1.ProductList
			fsys := artifacts.Products()
			_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				data, err := fs.ReadFile(fsys, path)
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				var p packv1alpha1.Product
				err = json.Unmarshal(data, &p)
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				if phase == "" || p.Spec.Phase == packv1alpha1.Phase(phase) {
					out.Items = append(out.Items, p)
				}
			})
			ctx.JSON(http.StatusOK, out)
		}))

		// PUBLIC
		m.Get("/products/{owner}/{key}", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products/appscode/kubedb
			// TODO: get product by (owner, key)

			data, err := fs.ReadFile(artifacts.Products(), ctx.R().Params("key")+".json")
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			_, _ = ctx.Write(data)
		}))

		// PUBLIC
		m.Get("/products/{owner}/{key}/plans", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products/appscode/kubedb
			// TODO: get product by (owner, key)

			dir := "artifacts/products/kubedb-plans"
			files, err := ioutil.ReadDir(dir)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			phase := ctx.R().Params("phase")
			var out packv1alpha1.PlanList
			for _, file := range files {
				data, err := ioutil.ReadFile(filepath.Join(dir, file.Name()))
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				var plaan packv1alpha1.Plan
				err = json.Unmarshal(data, &plaan)
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				if phase == "" || plaan.Spec.Phase == packv1alpha1.Phase(phase) {
					out.Items = append(out.Items, plaan)
				}
			}
			ctx.JSON(http.StatusOK, out)
		}))

		// PUBLIC
		m.Get("/products/{owner}/{key}/plans/{plan}", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products/appscode/kubedb
			// TODO: get product by (owner, key)

			dir := "artifacts/products/" + ctx.R().Params("key") + "-plans"

			data, err := ioutil.ReadFile(filepath.Join(dir, ctx.R().Params("plan")+".json"))
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			var plaan packv1alpha1.Plan
			err = json.Unmarshal(data, &plaan)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			ctx.JSON(http.StatusOK, plaan)
		}))

		// PUBLIC
		m.Get("/products/{owner}/{key}/compare", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products/appscode/kubedb
			// TODO: get product by (owner, key)

			phase := ctx.R().Params("phase")

			// product
			data, err := fs.ReadFile(artifacts.Products(), ctx.R().Params("key")+".json")
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			var p packv1alpha1.Product
			err = json.Unmarshal(data, &p)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			var url string
			version := p.Spec.LatestVersion
			var plaans []packv1alpha1.Plan

			dir := "artifacts/products/" + ctx.R().Params("key") + "-plans"
			files, err := ioutil.ReadDir(dir)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			for idx, file := range files {
				data, err := ioutil.ReadFile(filepath.Join(dir, file.Name()))
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				var plaan packv1alpha1.Plan
				err = json.Unmarshal(data, &plaan)
				if err != nil {
					ctx.Error(http.StatusInternalServerError, err.Error())
					return
				}
				if idx == 0 {
					url = plaan.Spec.Bundle.URL
				}

				if phase == "" || plaan.Spec.Phase == packv1alpha1.Phase(phase) {
					plaans = append(plaans, plaan)
				}
			}

			sort.Slice(plaans, func(i, j int) bool {
				if plaans[i].Spec.Weight == plaans[j].Spec.Weight {
					return plaans[i].Spec.NickName < plaans[j].Spec.NickName
				}
				return plaans[i].Spec.Weight < plaans[j].Spec.Weight
			})

			var names []string
			for _, plaan := range plaans {
				names = append(names, plaan.Spec.Bundle.Name)
			}

			table, err := lib.ComparePlans(lib.DefaultRegistry, url, names, version)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			table.Plans = plaans
			ctx.JSON(http.StatusOK, table)
		}))

		// PUBLIC
		m.Get("/products/{owner}/{key}/plans/{plan}/bundleview", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// /products/appscode/kubedb
			// TODO: get product by (owner, key)

			// product
			data, err := fs.ReadFile(artifacts.Products(), ctx.R().Params("key")+".json")
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			var p packv1alpha1.Product
			err = json.Unmarshal(data, &p)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			// plan
			dir := "artifacts/products/" + ctx.R().Params("key") + "-plans"
			data, err = ioutil.ReadFile(filepath.Join(dir, ctx.R().Params("plan")+".json"))
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			var plaan packv1alpha1.Plan
			err = json.Unmarshal(data, &plaan)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			bv, err := lib.CreateBundleViewForBundle(lib.DefaultRegistry, &packv1alpha1.ChartRepoRef{
				URL:     plaan.Spec.Bundle.URL,
				Name:    plaan.Spec.Bundle.Name,
				Version: p.Spec.LatestVersion,
			})
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			ctx.JSON(http.StatusOK, bv)
		}))

		// PUBLIC
		m.Get("/product_id/{id}", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			// TODO: get product by id

			data, err := fs.ReadFile(artifacts.Products(), "kubedb.json")
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}
			_, _ = ctx.Write(data)
		}))
	*/

	// PRIVATE
	// Should we store Order UID in a table per User?
	m.Route("/deploy/orders", func(m chi.Router) {
		// Generate Order for a single chart and optional values patch
		m.With(binding.JSON(packv1alpha1.Order{})).Post("/", binding.HandlerFunc(CreateOrder))
		m.Get("/{id}/render/manifest", binding.HandlerFunc(PreviewOrderManifest))
		m.Get("/{id}/render/resources", binding.HandlerFunc(PreviewOrderResources))
		m.Get("/{id}/helm3", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			bs, err := lib.NewTestBlobStore()
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			data, err := bs.ReadFile(ctx.R().Request().Context(), path.Join(ctx.R().Params("id"), "order.yaml"))
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			var order packv1alpha1.Order
			err = yaml.Unmarshal(data, &order)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			scripts, err := lib.GenerateHelm3Script(bs, lib.DefaultRegistry, order)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			ctx.JSON(http.StatusOK, scripts)
		}))
		m.Get("/{id}/yaml", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
			bs, err := lib.NewTestBlobStore()
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			data, err := bs.ReadFile(ctx.R().Request().Context(), path.Join(ctx.R().Params("id"), "order.yaml"))
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			var order packv1alpha1.Order
			err = yaml.Unmarshal(data, &order)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			scripts, err := lib.GenerateYAMLScript(bs, lib.DefaultRegistry, order)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, err.Error())
				return
			}

			ctx.JSON(http.StatusOK, scripts)
		}))
	})

	// PRIVATE
	m.Route("/clusters/{cluster}", func(m chi.Router) {
		m.Route("/editor", func(m chi.Router) {
			// create / update / apply / install
			m.With(binding.JSON(map[string]interface{}{})).Put("/", binding.HandlerFunc(ApplyResource(f)))

			m.Delete("/namespaces/{namespace}/releases/{releaseName}", binding.HandlerFunc(DeleteResource(f)))

			// POST Model from Existing Installations
			m.With(binding.JSON(appapi.ModelMetadata{})).Put("/model", binding.HandlerFunc(LoadEditorModel))

			// redundant apis
			// can be replaced by getting the model, then using the /editor apis
			m.With(binding.JSON(appapi.Model{})).Put("/manifest", binding.HandlerFunc(LoadEditorManifest))

			// redundant apis
			// can be replaced by getting the model, then using the /editor apis
			m.With(binding.JSON(appapi.Model{})).Put("/resources", binding.HandlerFunc(LoadEditorResources))
		})

		m.Post("/deploy/{id}", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
		}))
		m.Delete("/deploy/{id}", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
		}))
	})

	m.Get("/chartrepositories", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
		repos, err := repo.DefaultNamer.ListHelmHubRepositories()
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, repos)
	}))
	m.Get("/chartrepositories/charts", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
		url := ctx.R().Query("url")
		if url == "" {
			ctx.Error(http.StatusBadRequest, "missing url")
			return
		}

		cfg, _, err := lib.DefaultRegistry.Get(url)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		cr, err := repo.NewChartRepository(cfg, getter.All())
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		err = cr.Load()
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, cr.ListCharts())
	}))
	m.Get("/chartrepositories/charts/{name}/versions", binding.HandlerFunc(func(ctx httpw.ResponseWriter) {
		url := ctx.R().Query("url")
		name := ctx.R().Params("name")

		if url == "" {
			ctx.Error(http.StatusBadRequest, "missing url")
			return
		}
		if name == "" {
			ctx.Error(http.StatusBadRequest, "missing chart name")
			return
		}

		cfg, _, err := lib.DefaultRegistry.Get(url)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		cr, err := repo.NewChartRepository(cfg, getter.All())
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		err = cr.Load()
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, cr.ListVersions(name))
	}))

	m.Get("/", binding.HandlerFunc(func() string {
		return "Hello world!"
	}))
	klog.Infoln()
	klog.Infoln("Listening on :4000")
	if err := http.ListenAndServe(":4000", m); err != nil {
		klog.Fatalln(err)
	}
}

func LoadEditorResources(ctx httpw.ResponseWriter, model appapi.Model) {
	cfg, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	tpl, err := editor.LoadEditorModel(cfg, lib.DefaultRegistry, appapi.ModelMetadata{
		Metadata: model.Metadata,
	})
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "LoadEditorModel", err.Error())
		return
	}

	var out appapi.ResourceOutput
	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.YAMLFormat)

	for _, r := range tpl.Resources {
		data, err := meta_util.Marshal(r, format)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "MarshalResource", err.Error())
			return
		}
		out.Resources = append(out.Resources, appapi.ResourceFile{
			Filename: r.Filename + "." + string(format),
			Data:     string(data),
		})
	}
	ctx.JSON(http.StatusOK, out)
}

func LoadEditorManifest(ctx httpw.ResponseWriter, model appapi.Model) {
	cfg, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	tpl, err := editor.LoadEditorModel(cfg, lib.DefaultRegistry, appapi.ModelMetadata{
		Metadata: model.Metadata,
	})
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "LoadEditorModel", err.Error())
		return
	}
	_, _ = ctx.Write(tpl.Manifest)
}

func LoadEditorModel(ctx httpw.ResponseWriter, model appapi.ModelMetadata) {
	cfg, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	tpl, err := editor.LoadEditorModel(cfg, lib.DefaultRegistry, model)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "LoadEditorModel", err.Error())
		return
	}

	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.JsonFormat)
	out, err := meta_util.Marshal(tpl.Values, format)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "MarshalModel", err.Error())
		return
	}
	_, _ = ctx.Write(out)
}

func DeleteResource(f cmdutil.Factory) func(ctx httpw.ResponseWriter) {
	return func(ctx httpw.ResponseWriter) {
		release := appapi.ObjectMeta{
			Name:      ctx.R().Params("releaseName"),
			Namespace: ctx.R().Params("namespace"),
		}
		rls, err := handler.DeleteResource(f, release)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "DeleteResource", err.Error())
			return
		}
		_, _ = ctx.Write([]byte(rls.Info))
	}
}

func ApplyResource(f cmdutil.Factory) func(ctx httpw.ResponseWriter, model map[string]interface{}) {
	return func(ctx httpw.ResponseWriter, model map[string]interface{}) {
		rls, err := handler.ApplyResource(f, model, !ctx.R().QueryBool("installCRDs"))
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "ApplyResource", err.Error())
			return
		}
		ctx.JSON(http.StatusOK, rls.Info)
	}
}

func PreviewOrderResources(ctx httpw.ResponseWriter) {
	bs, err := lib.NewTestBlobStore()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	data, err := bs.ReadFile(ctx.R().Request().Context(), path.Join(ctx.R().Params("id"), "order.yaml"))
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "BlobStoreReadFile", err.Error())
		return
	}

	var order packv1alpha1.Order
	err = yaml.Unmarshal(data, &order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "UnmarshalOrder", err.Error())
		return
	}

	_, tpls, err := editor.RenderOrderTemplate(bs, lib.DefaultRegistry, order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "RenderOrderTemplate", err.Error())
		return
	}
	if ctx.R().QueryBool("skipCRDs") {
		for i := range tpls {
			tpls[i].CRDs = nil
		}
	}

	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.YAMLFormat)
	out, err := editor.ConvertChartTemplates(tpls, format)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "ConvertChartTemplates", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, out)
}

func PreviewOrderManifest(ctx httpw.ResponseWriter) {
	bs, err := lib.NewTestBlobStore()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	data, err := bs.ReadFile(ctx.R().Request().Context(), path.Join(ctx.R().Params("id"), "order.yaml"))
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "BlobStoreReadFile", err.Error())
		return
	}

	var order packv1alpha1.Order
	err = yaml.Unmarshal(data, &order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "UnmarshalOrder", err.Error())
		return
	}

	manifest, _, err := editor.RenderOrderTemplate(bs, lib.DefaultRegistry, order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "RenderOrderTemplate", err.Error())
		return
	}
	_, _ = ctx.Write([]byte(manifest))
}

func CreateOrder(ctx httpw.ResponseWriter, order packv1alpha1.Order) {
	if len(order.Spec.Packages) == 0 {
		ctx.Error(http.StatusBadRequest, "missing package selection for order")
		return
	}

	order.TypeMeta = metav1.TypeMeta{
		APIVersion: packv1alpha1.SchemeGroupVersion.String(),
		Kind:       packv1alpha1.ResourceKindOrder,
	}
	order.ObjectMeta = metav1.ObjectMeta{
		UID:               types.UID(uuid.New().String()),
		CreationTimestamp: metav1.NewTime(time.Now()),
	}
	if order.Name == "" {
		order.Name = order.Spec.Packages[0].Chart.ReleaseName
	}

	data, err := yaml.Marshal(order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "MarshalOrder", err.Error())
		return
	}

	bs, err := lib.NewTestBlobStore()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	err = bs.WriteFile(ctx.R().Request().Context(), path.Join(string(order.UID), "order.yaml"), data)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "BlobStoreWriteFile", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, order)
}

func PreviewEditorResources(ctx httpw.ResponseWriter, opts map[string]interface{}) {
	_, tpls, err := editor.RenderChartTemplate(lib.DefaultRegistry, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "RenderChartTemplate", err.Error())
		return
	}
	if ctx.R().QueryBool("skipCRDs") {
		tpls.CRDs = nil
	}

	var out appapi.ResourceOutput
	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.YAMLFormat)

	for _, crd := range tpls.CRDs {
		data, err := meta_util.Marshal(crd.Data, format)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "MarshalCRD", err.Error())
			return
		}
		out.CRDs = append(out.CRDs, appapi.ResourceFile{
			Filename: crd.Filename + "." + string(format),
			Data:     string(data),
		})
	}
	for _, r := range tpls.Resources {
		data, err := meta_util.Marshal(r.Data, format)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "MarshalResource", err.Error())
			return
		}
		out.Resources = append(out.Resources, appapi.ResourceFile{
			Filename: r.Filename + "." + string(format),
			Data:     string(data),
		})
	}
	ctx.JSON(http.StatusOK, &out)
}

func PreviewEditorManifest(ctx httpw.ResponseWriter, opts map[string]interface{}) {
	manifest, _, err := editor.RenderChartTemplate(lib.DefaultRegistry, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "RenderChartTemplate", err.Error())
		return
	}
	_, _ = ctx.Write([]byte(manifest))
}

func GenerateEditorModelFromOptions(ctx httpw.ResponseWriter, opts map[string]interface{}) {
	model, err := editor.GenerateEditorModel(lib.DefaultRegistry, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "GetChart", err.Error())
		return
	}

	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.JsonFormat)
	if format == meta_util.YAMLFormat {
		out, err := yaml.Marshal(model)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		_, _ = ctx.Write(out)
		return
	}
	ctx.JSON(http.StatusOK, model)
}

func GetPackageFile(ctx httpw.ResponseWriter, params packv1alpha1.ChartRepoRef) {
	chrt, err := lib.DefaultRegistry.GetChart(params.URL, params.Name, params.Version)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "GetChart", err.Error())
		return
	}

	filename := ctx.R().Params("*")
	format := meta_util.NewDataFormat(ctx.R().QueryTrim("format"), meta_util.KeepFormat)
	for _, f := range chrt.Raw {
		if f.Name == filename {
			out, ct, err := converter.Convert(f.Name, f.Data, format)
			if err != nil {
				ctx.Error(http.StatusInternalServerError, "ConvertFormat", err.Error())
				return
			}

			ctx.Header().Set("Content-Type", ct)
			_, _ = ctx.Write(out)
			return
		}
	}
	ctx.WriteHeader(http.StatusNotFound)
}

func ListPackageFiles(ctx httpw.ResponseWriter, params packv1alpha1.ChartRepoRef) {
	// TODO: verify params

	chrt, err := lib.DefaultRegistry.GetChart(params.URL, params.Name, params.Version)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "GetChart", err.Error())
		return
	}

	files := make([]string, 0, len(chrt.Raw))
	for _, f := range chrt.Raw {
		files = append(files, f.Name)
	}
	sort.Strings(files)

	ctx.JSON(http.StatusOK, files)
}

func CreateOrderForPackage(ctx httpw.ResponseWriter, params appapi.ChartOrder) {
	order, err := editor.CreateChartOrder(lib.DefaultRegistry, params)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CreateChartOrder", err.Error())
		return
	}

	data, err := yaml.Marshal(order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "MarshalOrder", err.Error())
		return
	}

	bs, err := lib.NewTestBlobStore()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	err = bs.WriteFile(ctx.R().Request().Context(), path.Join(string(order.UID), "order.yaml"), data)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "BlobStoreWriteFile", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, order)
}

func GetPackageViewForChart(ctx httpw.ResponseWriter, params packv1alpha1.ChartRepoRef) {
	// TODO: verify params

	chrt, err := lib.DefaultRegistry.GetChart(params.URL, params.Name, params.Version)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "GetChart", err.Error())
		return
	}

	pv, err := lib.CreatePackageView(params.URL, chrt.Chart)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CreatePackageView", err.Error())
		return
	}

	ctx.JSON(http.StatusOK, pv)
}

func CreateOrderForBundle(ctx httpw.ResponseWriter, params packv1alpha1.BundleView) {
	order, err := lib.CreateOrder(lib.DefaultRegistry, params)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CreateDeployOrder", err.Error())
		return
	}

	data, err := yaml.Marshal(order)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "MarshalDeployOrder", err.Error())
		return
	}

	bs, err := lib.NewTestBlobStore()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	err = bs.WriteFile(ctx.R().Request().Context(), path.Join(string(order.UID), "order.yaml"), data)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "BlobStoreWriteFile", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, order)
}

func GetBundleViewForChart(ctx httpw.ResponseWriter, params packv1alpha1.ChartRepoRef) {
	// TODO: verify params

	bv, err := lib.CreateBundleViewForChart(lib.DefaultRegistry, &params)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CreateBundleViewForChart", err.Error())
		return
	}

	ctx.JSON(http.StatusOK, bv)
}
