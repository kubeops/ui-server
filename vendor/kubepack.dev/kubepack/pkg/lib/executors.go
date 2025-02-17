/*
Copyright AppsCode Inc. and Contributors

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

package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	libchart "kubepack.dev/lib-helm/pkg/chart"
	"kubepack.dev/lib-helm/pkg/repo"
	"kubepack.dev/lib-helm/pkg/values"

	"github.com/Masterminds/semver/v3"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	authorization "k8s.io/api/authorization/v1"
	core "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"kmodules.xyz/client-go/apiextensions"
	disco_util "kmodules.xyz/client-go/discovery"
	"kmodules.xyz/client-go/tools/parser"
	wait2 "kmodules.xyz/client-go/tools/wait"
	"kmodules.xyz/resource-metadata/hub"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yamllib "sigs.k8s.io/yaml"
	"x-helm.dev/apimachinery/apis"
	driversapi "x-helm.dev/apimachinery/apis/drivers/v1alpha1"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

type DoFn func() error

type WaitForPrinter struct {
	Name      string
	Namespace string
	WaitFors  []releasesapi.WaitFlags
	W         io.Writer
}

func (x *WaitForPrinter) Do() error {
	if len(x.WaitFors) == 0 {
		return nil
	}

	_, err := fmt.Fprintf(x.W, "# wait %s to be ready\n", x.Name)
	if err != nil {
		return err
	}
	for _, w := range x.WaitFors {
		// kubectl wait ([-f FILENAME] | resource.group/resource.name | resource.group [(-l label | --all)]) [--for=delete|--for condition=available] [options]

		parts := make([]string, 0, 10)
		parts = append(parts, "kubectl")
		parts = append(parts, "wait")

		if w.Resource.Group != "" {
			if w.Resource.Resource != "" {
				parts = append(parts, w.Resource.Group+"/"+w.Resource.Resource)
			} else {
				parts = append(parts, w.Resource.Group)
			}
		}

		if w.Labels != nil {
			parts = append(parts, "-l")

			selector, err := v1.LabelSelectorAsSelector(w.Labels)
			if err != nil {
				return err
			}
			parts = append(parts, selector.String())
		}

		if w.All {
			parts = append(parts, "--all")
		}

		if w.ForCondition != "" {
			parts = append(parts, "--for")
			parts = append(parts, w.ForCondition)
		}

		if w.Timeout.Duration > 0 {
			parts = append(parts, "--timeout")
			parts = append(parts, w.Timeout.Duration.String())
		}

		if x.Namespace != "" {
			parts = append(parts, "-n")
			parts = append(parts, x.Namespace)
		}

		_, err = fmt.Fprintln(x.W, strings.Join(parts, " "))
		if err != nil {
			return err
		}
	}
	return nil
}

type WaitForChecker struct {
	Namespace string
	WaitFors  []releasesapi.WaitFlags

	ClientGetter genericclioptions.RESTClientGetter
}

func (x *WaitForChecker) Do() error {
	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	for _, flags := range x.WaitFors {
		builder := resource.NewBuilder(x.ClientGetter).
			NamespaceParam(x.Namespace).DefaultNamespace().
			// AllNamespaces(true).
			Unstructured().
			Latest().
			ContinueOnError().
			Flatten()
		if flags.All {
			builder.ResourceTypeOrNameArgs(true, flags.Resource.Group)
		} else {
			builder.ResourceTypeOrNameArgs(false, flags.Resource.Group+"/"+flags.Resource.Resource)
		}
		if flags.Labels != nil {
			selector, err := v1.LabelSelectorAsSelector(flags.Labels)
			if err != nil {
				return err
			}
			builder.LabelSelectorParam(selector.String())
		}

		clientConfig, err := x.ClientGetter.ToRESTConfig()
		if err != nil {
			return err
		}
		dynamicClient, err := dynamic.NewForConfig(clientConfig)
		if err != nil {
			return err
		}
		conditionFn, err := wait2.ConditionFuncFor(flags.ForCondition, streams.ErrOut)
		if err != nil {
			return err
		}

		effectiveTimeout := flags.Timeout.Duration
		if effectiveTimeout < 0 {
			effectiveTimeout = 168 * time.Hour
		}

		o := &wait2.WaitOptions{
			ResourceFinder: &ResourceFindBuilderWrapper{builder},
			DynamicClient:  dynamicClient,
			Timeout:        effectiveTimeout,

			Printer:     printers.NewDiscardingPrinter(),
			ConditionFn: conditionFn,
			IOStreams:   streams,
		}

		err = o.WaitUntilAvailable(flags.ForCondition)
		if err != nil {
			return err
		}
		err = o.RunWait()
		if err != nil {
			return err
		}
	}
	return nil
}

// ResourceFindBuilderWrapper wraps a builder in an interface
type ResourceFindBuilderWrapper struct {
	builder *resource.Builder
}

// Do finds you resources to check
func (b *ResourceFindBuilderWrapper) Do() resource.Visitor {
	return b.builder.Do()
}

type CRDReadinessPrinter struct {
	CRDs []metav1.GroupVersionResource
	W    io.Writer
}

func (x *CRDReadinessPrinter) Do() error {
	_, err := fmt.Fprintln(x.W, "# wait for crds to be ready")
	if err != nil {
		return err
	}

	for _, crd := range x.CRDs {
		// Work around for bug: https://github.com/kubernetes/kubernetes/issues/83242
		_, err := fmt.Fprintf(x.W, "until kubectl get crds %s.%s -o=jsonpath='{.items[0].metadata.name}' &> /dev/null; do sleep 1; done\n", crd.Resource, crd.Group)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(x.W, "kubectl wait --for=condition=Established crds/%s.%s --timeout=5m\n", crd.Resource, crd.Group)
		if err != nil {
			return err
		}
	}
	return nil
}

type CRDReadinessChecker struct {
	CRDs   []metav1.GroupVersionResource
	Client crd_cs.Interface
}

func (x *CRDReadinessChecker) Do() error {
	crds := make([]*apiextensions.CustomResourceDefinition, 0, len(x.CRDs))
	for _, crd := range x.CRDs {
		crds = append(crds, &apiextensions.CustomResourceDefinition{
			V1: &crdv1.CustomResourceDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: fmt.Sprintf("%s.%s", crd.Resource, crd.Group),
				},
				Spec: crdv1.CustomResourceDefinitionSpec{
					Group: crd.Group,
					Names: crdv1.CustomResourceDefinitionNames{
						Plural: crd.Resource,
						// Kind:   crd.Kind,
					},
					// Scope: crdv1beta1.ResourceScope(string(crd.Scope)),
					Versions: []crdv1.CustomResourceDefinitionVersion{
						{
							Name: crd.Version,
						},
					},
				},
			},
		})
	}
	return apiextensions.WaitForCRDReady(x.Client, crds)
}

type Helm3CommandPrinter struct {
	Registry      repo.IRegistry
	ChartRef      releasesapi.ChartRef
	Version       string
	ReleaseName   string
	Namespace     string
	Values        values.Options
	UseValuesFile bool

	W          io.Writer
	valuesFile []byte
}

const indent = "  "

func (x *Helm3CommandPrinter) Do() error {
	chrt, err := x.Registry.GetChart(releasesapi.ChartSourceRef{
		Name:      x.ChartRef.Name,
		Version:   x.Version,
		SourceRef: x.ChartRef.SourceRef,
	})
	if err != nil {
		return err
	}

	repoURL := x.ChartRef.SourceRef.Name
	switch x.ChartRef.SourceRef.Kind {
	case releasesapi.SourceKindHelmRepository:
		helmRepo, err := x.Registry.GetHelmRepository(releasesapi.ChartSourceRef{
			Name:      x.ChartRef.Name,
			Version:   x.Version,
			SourceRef: x.ChartRef.SourceRef,
		})
		if err != nil {
			return err
		}
		repoURL = helmRepo.Spec.URL
	}

	/*
		$ helm repo add appscode https://charts.appscode.com/stable/
		$ helm repo update
		$ helm search repo appscode/voyager --version v12.0.0-rc.1
	*/

	var buf bytes.Buffer
	if !registry.IsOCI(repoURL) {
		/*
			$ helm upgrade --install voyager-operator appscode/voyager --version v12.0.0-rc.1 \
			  --namespace kube-system \
			  --set cloudProvider=$provider
		*/
		_, err = fmt.Fprintf(&buf, "helm upgrade --install %s %s \\\n", x.ReleaseName, x.ChartRef.Name)
		if err != nil {
			return err
		}

		if x.Version != "" {
			_, err = fmt.Fprintf(&buf, "%s--repo %s --version %s \\\n", indent, repoURL, x.Version)
			if err != nil {
				return err
			}
		} else {
			_, err = fmt.Fprintf(&buf, "%s--repo %s \\\n", indent, repoURL)
			if err != nil {
				return err
			}
		}
	} else {
		u, err := url.Parse(repoURL)
		if err != nil {
			return err
		}
		u.Path = path.Join(u.Path, x.ChartRef.Name)
		u.User = nil
		repoURL = u.String()

		if x.Version != "" {
			_, err = fmt.Fprintf(&buf, "helm upgrade --install %s %s --version %s \\\n", x.ReleaseName, repoURL, x.Version)
			if err != nil {
				return err
			}
		} else {
			_, err = fmt.Fprintf(&buf, "helm upgrade --install %s %s \\\n", x.ReleaseName, repoURL)
			if err != nil {
				return err
			}
		}
	}

	if x.Namespace != "" {
		_, err = fmt.Fprintf(&buf, "%s--namespace %s --create-namespace \\\n", indent, x.Namespace)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(&buf, "%s--wait --debug --burst-limit=1000 \\\n", indent)
	if err != nil {
		return err
	}

	modified, err := x.Values.MergeValues(chrt.Chart)
	if err != nil {
		return err
	}
	if x.UseValuesFile {
		x.valuesFile, err = values.GetValuesDiffYAML(chrt.Values, modified)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(&buf, "%s--values=values.yaml", indent)
		if err != nil {
			return err
		}
	} else {
		setValues, err := values.GetChangedValues(chrt.Values, modified)
		if err != nil {
			return err
		}
		for _, v := range setValues {
			// xref: https://github.com/kubepack/lib-helm/issues/72
			if strings.ContainsRune(v, '\n') {
				idx := strings.IndexRune(v, '=')
				return fmt.Errorf(`found \n is values for %s`, v[:idx])
			}
			_, err = fmt.Fprintf(&buf, "%s--set %s \\\n", indent, v)
			if err != nil {
				return err
			}
		}
		buf.Truncate(buf.Len() - 3)
	}

	_, err = buf.WriteRune('\n')
	if err != nil {
		return err
	}

	_, err = buf.WriteTo(x.W)
	return err
}

func (x *Helm3CommandPrinter) ValuesFile() []byte {
	return x.valuesFile
}

type YAMLPrinter struct {
	Registry    repo.IRegistry
	ChartRef    releasesapi.ChartRef
	Version     string
	ReleaseName string
	Namespace   string
	KubeVersion string
	ValuesFile  string
	ValuesPatch *runtime.RawExtension

	BucketURL string
	UID       string
	PublicURL string
	Prefix    string
	W         io.Writer
}

func (x *YAMLPrinter) Do() error {
	ctx := context.Background()
	bucket, err := blob.OpenBucket(ctx, x.BucketURL)
	if err != nil {
		return err
	}

	if x.Prefix != "" {
		bucket = blob.PrefixedBucket(bucket, strings.TrimSuffix(x.Prefix, "/")+"/")
	}
	dirManifest := blob.PrefixedBucket(bucket, x.UID+"/manifests/")
	defer dirManifest.Close()
	dirCRD := blob.PrefixedBucket(bucket, x.UID+"/crds/")
	defer dirCRD.Close()

	var buf bytes.Buffer

	chrt, err := x.Registry.GetChart(releasesapi.ChartSourceRef{
		Name:      x.ChartRef.Name,
		Version:   x.Version,
		SourceRef: x.ChartRef.SourceRef,
	})
	if err != nil {
		return err
	}

	cfg := new(action.Configuration)
	client := action.NewInstall(cfg)
	var extraAPIs []string

	client.DryRun = true
	client.ReleaseName = x.ReleaseName
	client.Namespace = x.Namespace
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.APIVersions = chartutil.VersionSet(extraAPIs)
	client.Version = x.Version

	validInstallableChart, err := libchart.IsChartInstallable(chrt.Chart)
	if !validInstallableChart {
		return err
	}

	if chrt.Metadata.Deprecated {
		_, err = fmt.Fprintln(&buf, "# WARNING: This chart is deprecated")
		if err != nil {
			return err
		}

	}

	if req := chrt.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(chrt.Chart, req); err != nil {
			return err
		}
	}

	vals := chrt.Values
	if x.ValuesPatch != nil {
		if x.ValuesFile != "" && x.ValuesFile != chartutil.ValuesfileName {
			for _, f := range chrt.Raw {
				if f.Name == x.ValuesFile {
					if err := yamllib.Unmarshal(f.Data, &vals); err != nil {
						return fmt.Errorf("cannot load %s. Reason: %v", f.Name, err.Error())
					}
					break
				}
			}
		}
		values, err := json.Marshal(vals)
		if err != nil {
			return err
		}

		patchData, err := json.Marshal(x.ValuesPatch)
		if err != nil {
			return err
		}
		patch, err := jsonpatch.DecodePatch(patchData)
		if err != nil {
			return err
		}
		modifiedValues, err := patch.Apply(values)
		if err != nil {
			return err
		}
		err = json.Unmarshal(modifiedValues, &vals)
		if err != nil {
			return err
		}
	}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	if crds := chrt.CRDObjects(); len(crds) > 0 {
		_, err = fmt.Fprintln(&buf, "# install CRDs")
		if err != nil {
			return err
		}

		for _, crd := range crds {
			// Open the key "${releaseName}.yaml" for writing with the default options.
			w, err := dirCRD.NewWriter(ctx, crd.Name, nil)
			if err != nil {
				return err
			}
			_, writeErr := w.Write(crd.File.Data)
			// Always check the return value of Close when writing.
			closeErr := w.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}

			_, err = fmt.Fprintf(&buf, "kubectl apply -f %s\n", x.PublicURL+"/"+path.Join(x.UID, "crds", crd.Name))
			if err != nil {
				return err
			}
		}
	}

	if err := chartutil.ProcessDependencies(chrt.Chart, vals); err != nil {
		return err
	}

	caps := chartutil.DefaultCapabilities
	if x.KubeVersion != "" {
		infoPtr, err := semver.NewVersion(x.KubeVersion)
		if err != nil {
			return err
		}
		info := *infoPtr
		info, _ = info.SetPrerelease("")
		info, _ = info.SetMetadata("")
		caps.KubeVersion = chartutil.KubeVersion{
			Version: info.Original(),
			Major:   strconv.FormatUint(info.Major(), 10),
			Minor:   strconv.FormatUint(info.Minor(), 10),
		}
	}
	options := chartutil.ReleaseOptions{
		Name:      x.ReleaseName,
		Namespace: x.Namespace,
		Revision:  1,
		IsInstall: true,
	}
	valuesToRender, err := chartutil.ToRenderValues(chrt.Chart, vals, options, caps)
	if err != nil {
		return err
	}

	hooks, manifests, err := libchart.RenderResources(chrt.Chart, caps, valuesToRender)
	if err != nil {
		return err
	}

	var manifestDoc bytes.Buffer

	if !apis.BuiltinNamespaces.Has(x.Namespace) {
		manifestDoc.WriteString(fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s

`, x.Namespace))
	}

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPreInstall) {
			// TODO: Mark as pre-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}

	for _, m := range manifests {
		_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", m.Name, m.Content)
		if err != nil {
			return err
		}
	}

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPostInstall) {
			// TODO: Mark as post-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}

	{
		// Open the key "${releaseName}.yaml" for writing with the default options.
		w, err := dirManifest.NewWriter(ctx, x.ReleaseName+".yaml", nil)
		if err != nil {
			return err
		}
		_, writeErr := manifestDoc.WriteTo(w)
		// Always check the return value of Close when writing.
		closeErr := w.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}

		_, err = fmt.Fprintf(&buf, "kubectl apply -f %s\n", x.PublicURL+"/"+path.Join(x.UID, "manifests", x.ReleaseName+".yaml"))
		if err != nil {
			return err
		}
	}

	_, err = buf.WriteTo(x.W)
	return err
}

func debug(format string, v ...interface{}) {
	format = fmt.Sprintf("[debug] %s\n", format)
	_ = log.Output(2, fmt.Sprintf(format, v...))
}

type ResourcePermission struct {
	Items   []*unstructured.Unstructured
	Allowed bool
}

type PermissionChecker struct {
	Registry    repo.IRegistry
	ChartRef    releasesapi.ChartRef
	Version     string
	ReleaseName string
	Namespace   string
	Verb        string

	Config       *rest.Config
	ClientGetter genericclioptions.RESTClientGetter
	Mapper       disco_util.ResourceMapper

	attrs map[authorization.ResourceAttributes]*ResourcePermission
	m     sync.Mutex
}

func (x *PermissionChecker) Do() error {
	if x.attrs == nil {
		x.attrs = make(map[authorization.ResourceAttributes]*ResourcePermission)
	}

	chrt, err := x.Registry.GetChart(releasesapi.ChartSourceRef{
		Name:      x.ChartRef.Name,
		Version:   x.Version,
		SourceRef: x.ChartRef.SourceRef,
	})
	if err != nil {
		return err
	}

	cfg := new(action.Configuration)
	err = cfg.Init(x.ClientGetter, x.Namespace, "memory", debug)
	if err != nil {
		return err
	}

	client := action.NewInstall(cfg)
	var extraAPIs []string

	client.DryRun = true
	client.ReleaseName = x.ReleaseName
	client.Namespace = x.Namespace
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.APIVersions = chartutil.VersionSet(extraAPIs)
	client.Version = x.Version

	validInstallableChart, err := libchart.IsChartInstallable(chrt.Chart)
	if !validInstallableChart {
		return err
	}

	//if chrt.Metadata.Deprecated {
	//	_, err = fmt.Fprintln(&buf, "# WARNING: This chart is deprecated")
	//	if err != nil {
	//		return err
	//	}
	//
	//}

	if req := chrt.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(chrt.Chart, req); err != nil {
			return err
		}
	}

	vals := chrt.Values
	//if x.ValuesPatch != nil {
	//	values, err := json.Marshal(chrt.Values)
	//	if err != nil {
	//		return err
	//	}
	//
	//	patchData, err := json.Marshal(x.ValuesPatch)
	//	if err != nil {
	//		return err
	//	}
	//	patch, err := jsonpatch.DecodePatch(patchData)
	//	if err != nil {
	//		return err
	//	}
	//	modifiedValues, err := patch.Apply(values)
	//	if err != nil {
	//		return err
	//	}
	//	err = json.Unmarshal(modifiedValues, &vals)
	//	if err != nil {
	//		return err
	//	}
	//}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	if crds := chrt.CRDObjects(); len(crds) > 0 {
		attr := authorization.ResourceAttributes{
			Verb:     x.Verb,
			Group:    "apiextensions.k8s.io",
			Version:  "v1beta1",
			Resource: "CustomResourceDefinition",
		}
		info, found := x.attrs[attr]
		if !found {
			info = new(ResourcePermission)
			x.attrs[attr] = info
		}

		for _, crd := range crds {
			items, err := parser.ListResources(crd.File.Data)
			if err != nil {
				return err
			}
			for i := range items {
				info.Items = append(info.Items, items[i].Object)
			}
		}
	}

	if err := chartutil.ProcessDependencies(chrt.Chart, vals); err != nil {
		return err
	}

	caps, err := cfg.GetCapabilities()
	if err != nil {
		return err
	}
	options := chartutil.ReleaseOptions{
		Name:      x.ReleaseName,
		Namespace: x.Namespace,
		Revision:  1,
		IsInstall: true,
	}
	valuesToRender, err := chartutil.ToRenderValues(chrt.Chart, vals, options, caps)
	if err != nil {
		return err
	}

	hooks, manifests, err := libchart.RenderResources(chrt.Chart, caps, valuesToRender)
	if err != nil {
		return err
	}

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPreInstall) {
			err = ExtractResourceAttributes([]byte(hook.Manifest), x.Verb, x.Mapper, x.attrs)
			if err != nil {
				return err
			}
		}
	}

	for _, m := range manifests {
		err = ExtractResourceAttributes([]byte(m.Content), x.Verb, x.Mapper, x.attrs)
		if err != nil {
			return err
		}
	}

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPostInstall) {
			err = ExtractResourceAttributes([]byte(hook.Manifest), x.Verb, x.Mapper, x.attrs)
			if err != nil {
				return err
			}
		}
	}

	ac, err := authv1client.NewForConfig(x.Config)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for attr := range x.attrs {
		wg.Add(1)
		go func(attr authorization.ResourceAttributes) {
			defer wg.Done()

			result, err := ac.SelfSubjectAccessReviews().Create(context.TODO(), &authorization.SelfSubjectAccessReview{
				Spec: authorization.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &attr,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				panic(err) // TODO: return err
			}

			x.m.Lock()
			x.attrs[attr].Allowed = result.Status.Allowed
			x.m.Unlock()
		}(attr)
	}
	wg.Wait()

	return nil
}

func (x *PermissionChecker) Result() (map[authorization.ResourceAttributes]*ResourcePermission, bool) {
	for _, v := range x.attrs {
		if !v.Allowed {
			return x.attrs, false
		}
	}
	return x.attrs, true
}

type AppReleaseCRDRegPrinter struct {
	W io.Writer
}

func (x *AppReleaseCRDRegPrinter) Do() error {
	_, err := fmt.Fprintln(x.W, "kubectl apply -f https://github.com/x-helm/apimachinery/raw/master/crds/drivers.x-helm.dev_appreleases.yaml")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(x.W, "kubectl wait --for=condition=Established crds/appreleases.drivers.x-helm.dev --timeout=5m")
	if err != nil {
		return err
	}
	return nil
}

type AppReleaseCRDRegistrar struct {
	Config *rest.Config
}

func (x *AppReleaseCRDRegistrar) Do() error {
	apiextClient, err := crd_cs.NewForConfig(x.Config)
	if err != nil {
		return err
	}
	return apiextensions.RegisterCRDs(apiextClient, []*apiextensions.CustomResourceDefinition{
		driversapi.AppRelease{}.CustomResourceDefinition(),
	})
}

type ApplicationUploader struct {
	App       *driversapi.AppRelease
	UID       string
	BucketURL string
	PublicURL string
	Prefix    string
	W         io.Writer
}

func (x *ApplicationUploader) Do() error {
	ctx := context.Background()
	bucket, err := blob.OpenBucket(ctx, x.BucketURL)
	if err != nil {
		return err
	}

	if x.Prefix != "" {
		bucket = blob.PrefixedBucket(bucket, strings.TrimSuffix(x.Prefix, "/")+"/")
	}
	bucket = blob.PrefixedBucket(bucket, x.UID+"/apps/"+x.App.Namespace+"/")
	defer bucket.Close()

	data, err := yamllib.Marshal(x.App)
	if err != nil {
		return err
	}

	w, err := bucket.NewWriter(ctx, x.App.Name+".yaml", nil)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintln(w, string(data))
	// Always check the return value of Close when writing.
	closeErr := w.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	_, err = fmt.Fprintf(x.W, "kubectl apply -f %s/%s\n", x.PublicURL, path.Join(x.UID, "apps", x.App.Namespace, x.App.Name+".yaml"))
	if err != nil {
		return err
	}
	return nil
}

type ApplicationCreator struct {
	App    *driversapi.AppRelease
	Client client.Client
}

func (x *ApplicationCreator) Do() error {
	err := x.Client.Create(context.TODO(), x.App)
	return err
}

type ApplicationGenerator struct {
	Registry repo.IRegistry
	Chart    releasesapi.ChartSelection
	chrt     *chart.Chart

	KubeVersion string

	components   map[metav1.GroupVersionKind]struct{}
	commonLabels map[string]string
}

func (x *ApplicationGenerator) Do() error {
	if x.components == nil {
		x.components = make(map[metav1.GroupVersionKind]struct{})
	}
	if x.commonLabels == nil {
		x.commonLabels = make(map[string]string)
	}

	chrt, err := x.Registry.GetChart(releasesapi.ChartSourceRef{
		Name:      x.Chart.Name,
		Version:   x.Chart.Version,
		SourceRef: x.Chart.SourceRef,
	})
	x.chrt = chrt.Chart
	if err != nil {
		return err
	}

	cfg := new(action.Configuration)
	//err = cfg.Init(x.ClientGetter, x.Namespace, "memory", debug)
	//if err != nil {
	//	return err
	//}

	client := action.NewInstall(cfg)
	var extraAPIs []string

	client.DryRun = true
	client.ReleaseName = x.Chart.ReleaseName
	client.Namespace = x.Chart.Namespace
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.APIVersions = chartutil.VersionSet(extraAPIs)
	client.Version = x.Chart.Version

	validInstallableChart, err := libchart.IsChartInstallable(x.chrt)
	if !validInstallableChart {
		return err
	}

	//if chrt.Metadata.Deprecated {
	//	_, err = fmt.Fprintln(&buf, "# WARNING: This chart is deprecated")
	//	if err != nil {
	//		return err
	//	}
	//
	//}

	if req := x.chrt.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(x.chrt, req); err != nil {
			return err
		}
	}

	vals := x.chrt.Values
	//if x.ValuesPatch != nil {
	//	values, err := json.Marshal(chrt.Values)
	//	if err != nil {
	//		return err
	//	}
	//
	//	patchData, err := json.Marshal(x.ValuesPatch)
	//	if err != nil {
	//		return err
	//	}
	//	patch, err := jsonpatch.DecodePatch(patchData)
	//	if err != nil {
	//		return err
	//	}
	//	modifiedValues, err := patch.Apply(values)
	//	if err != nil {
	//		return err
	//	}
	//	err = json.Unmarshal(modifiedValues, &vals)
	//	if err != nil {
	//		return err
	//	}
	//}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	//if crds := chrt.CRDObjects(); len(crds) > 0 {
	//	attr := metav1.GroupKind{
	//		Group:    "apiextensions.k8s.io",
	//		Kind: "CustomResourceDefinition",
	//	}
	//	x.components[attr] = struct{}{}
	//}

	if err := chartutil.ProcessDependencies(x.chrt, vals); err != nil {
		return err
	}

	caps := chartutil.DefaultCapabilities
	if x.KubeVersion != "" {
		infoPtr, err := semver.NewVersion(x.KubeVersion)
		if err != nil {
			return err
		}
		info := *infoPtr
		info, _ = info.SetPrerelease("")
		info, _ = info.SetMetadata("")
		caps.KubeVersion = chartutil.KubeVersion{
			Version: info.Original(),
			Major:   strconv.FormatUint(info.Major(), 10),
			Minor:   strconv.FormatUint(info.Minor(), 10),
		}
	}
	options := chartutil.ReleaseOptions{
		Name:      x.Chart.ReleaseName,
		Namespace: x.Chart.Namespace,
		Revision:  1,
		IsInstall: true,
	}
	valuesToRender, err := chartutil.ToRenderValues(x.chrt, vals, options, caps)
	if err != nil {
		return err
	}

	hooks, manifests, err := libchart.RenderResources(x.chrt, caps, valuesToRender)
	if err != nil {
		return err
	}

	var manifestDoc bytes.Buffer
	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPreInstall) {
			// TODO: Mark as pre-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}
	for _, m := range manifests {
		_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", m.Name, m.Content)
		if err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPostInstall) {
			// TODO: Mark as post-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}
	x.components, x.commonLabels, err = parser.ExtractComponentGVKs(manifestDoc.Bytes())
	return err
}

func (x *ApplicationGenerator) Result() *driversapi.AppRelease {
	desc := GetPackageDescriptor(x.chrt)

	b := &driversapi.AppRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: releasesapi.GroupVersion.String(),
			Kind:       "AppRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      x.Chart.ReleaseName,
			Namespace: x.Chart.Namespace,
			Labels:    nil, // TODO: ?
			Annotations: map[string]string{
				apis.LabelChartURL:     x.Chart.SourceRef.Namespace + "/" + x.Chart.SourceRef.Name,
				apis.LabelChartName:    x.Chart.Name,
				apis.LabelChartVersion: x.Chart.Version,
			},
		},
		Spec: driversapi.AppReleaseSpec{
			Descriptor: driversapi.Descriptor{
				Type:        x.chrt.Name(),
				Description: desc.Description,
				Icons:       desc.Icons,
				Maintainers: desc.Maintainers,
				Keywords:    desc.Keywords,
				Links:       desc.Links,
				Notes:       "",
				Version:     x.chrt.Metadata.AppVersion,
				Owners:      nil, // TODO: Add the user email who is installing this app
			},
		},
	}

	gvks := make([]metav1.GroupVersionKind, 0, len(x.components))
	for gk := range x.components {
		gvks = append(gvks, gk)
	}
	sort.Slice(gvks, func(i, j int) bool {
		if gvks[i].Group == gvks[j].Group {
			return gvks[i].Kind < gvks[j].Kind
		}
		return gvks[i].Group < gvks[j].Group
	})
	b.Spec.Components = gvks

	if len(x.commonLabels) > 0 {
		b.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: x.commonLabels,
		}
	}
	return b
}

func ExtractResourceAttributes(data []byte, verb string, mapper disco_util.ResourceMapper, attrs map[authorization.ResourceAttributes]*ResourcePermission) error {
	return parser.ProcessResources(data, func(ri parser.ResourceInfo) error {
		gvr, err := mapper.GVR(schema.FromAPIVersionAndKind(ri.Object.GetAPIVersion(), ri.Object.GetKind()))
		if err != nil {
			return err
		}

		ns := XorY(ri.Object.GetNamespace(), core.NamespaceDefault)
		ri.Object.SetNamespace(ns)

		attr := authorization.ResourceAttributes{
			Namespace: ns,
			Verb:      verb,
			Group:     gvr.Group,
			Version:   gvr.Version,
			Resource:  gvr.Resource,
			// Name:      ri.GetName(), // TODO: needed for delete
		}
		info, found := attrs[attr]
		if !found {
			info = new(ResourcePermission)
			attrs[attr] = info
		}
		info.Items = append(info.Items, ri.Object)

		return nil
	})
}

type ChartRenderer struct {
	Registry repo.IRegistry
	releasesapi.ChartSourceRef
	ReleaseName string
	Namespace   string
	KubeVersion string
	ValuesFile  string
	ValuesPatch *runtime.RawExtension
	Values      map[string]interface{}

	CRDs               []chart.File
	Manifest           *chart.File
	IsFeaturesetEditor bool
}

func (x *ChartRenderer) Do() error {
	chrt, err := x.Registry.GetChart(x.ChartSourceRef)
	if err != nil {
		return err
	}

	if data, ok := chrt.Chart.Metadata.Annotations["meta.x-helm.dev/editor"]; ok && data != "" {
		var gvr metav1.GroupVersionResource
		if err := json.Unmarshal([]byte(data), &gvr); err != nil {
			return fmt.Errorf("failed to parse %s annotation %s", "meta.x-helm.dev/editor", data)
		}
		x.IsFeaturesetEditor = hub.IsFeaturesetGR(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource})
	}

	cfg := new(action.Configuration)
	client := action.NewInstall(cfg)
	var extraAPIs []string

	client.DryRun = true
	client.ReleaseName = x.ReleaseName
	client.Namespace = x.Namespace
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.APIVersions = extraAPIs
	client.Version = x.Version

	validInstallableChart, err := libchart.IsChartInstallable(chrt.Chart)
	if !validInstallableChart {
		return err
	}

	//if chrt.Metadata.Deprecated {
	//}

	if req := chrt.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(chrt.Chart, req); err != nil {
			return err
		}
	}

	vals := chrt.Values
	if x.ValuesPatch != nil {
		if x.ValuesFile != "" {
			for _, f := range chrt.Raw {
				if f.Name == x.ValuesFile {
					if err := yamllib.Unmarshal(f.Data, &vals); err != nil {
						return fmt.Errorf("cannot load %s. Reason: %v", f.Name, err.Error())
					}
					break
				}
			}
		}
		values, err := json.Marshal(vals)
		if err != nil {
			return err
		}

		patchData, err := json.Marshal(x.ValuesPatch)
		if err != nil {
			return err
		}
		patch, err := jsonpatch.DecodePatch(patchData)
		if err != nil {
			return err
		}
		modifiedValues, err := patch.Apply(values)
		if err != nil {
			return err
		}
		err = json.Unmarshal(modifiedValues, &vals)
		if err != nil {
			return err
		}
	} else if x.Values != nil {
		vals = x.Values
	}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	if crds := chrt.CRDObjects(); len(crds) > 0 {
		for _, crd := range crds {
			x.CRDs = append(x.CRDs, chart.File{
				Name: crd.Filename,
				Data: crd.File.Data,
			})
		}
	}

	if err := chartutil.ProcessDependencies(chrt.Chart, vals); err != nil {
		return err
	}

	caps := chartutil.DefaultCapabilities
	if x.KubeVersion != "" {
		infoPtr, err := semver.NewVersion(x.KubeVersion)
		if err != nil {
			return err
		}
		info := *infoPtr
		info, _ = info.SetPrerelease("")
		info, _ = info.SetMetadata("")
		caps.KubeVersion = chartutil.KubeVersion{
			Version: info.Original(),
			Major:   strconv.FormatUint(info.Major(), 10),
			Minor:   strconv.FormatUint(info.Minor(), 10),
		}
	}
	options := chartutil.ReleaseOptions{
		Name:      x.ReleaseName,
		Namespace: x.Namespace,
		Revision:  1,
		IsInstall: true,
	}
	valuesToRender, err := chartutil.ToRenderValues(chrt.Chart, vals, options, caps)
	if err != nil {
		return err
	}
	if x.Values != nil {
		valuesToRender["Values"] = x.Values
	}

	hooks, manifests, err := libchart.RenderResources(chrt.Chart, caps, valuesToRender)
	if err != nil {
		return err
	}

	var manifestDoc bytes.Buffer

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPreInstall) {
			// TODO: Mark as pre-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}

	for _, m := range manifests {
		_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", m.Name, m.Content)
		if err != nil {
			return err
		}
	}

	for _, hook := range hooks {
		if libchart.IsEvent(hook.Events, release.HookPostInstall) {
			// TODO: Mark as post-install hook
			_, err = fmt.Fprintf(&manifestDoc, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
			if err != nil {
				return err
			}
		}
	}

	{
		x.Manifest = &chart.File{
			Name: "manifest.yaml",
			Data: manifestDoc.Bytes(),
		}
	}

	return nil
}

func (x *ChartRenderer) Result() (crds []chart.File, manifest *chart.File) {
	crds = x.CRDs
	manifest = x.Manifest

	return
}
