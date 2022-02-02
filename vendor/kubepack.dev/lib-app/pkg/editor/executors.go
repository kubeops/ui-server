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

package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"kubepack.dev/kubepack/apis/kubepack/v1alpha1"
	appapi "kubepack.dev/lib-app/api/v1alpha1"
	libchart "kubepack.dev/lib-helm/pkg/chart"
	"kubepack.dev/lib-helm/pkg/repo"

	"github.com/Masterminds/semver/v3"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/gobuffalo/flect"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kmodules.xyz/client-go/discovery"
	"kmodules.xyz/resource-metadata/hub"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	yamllib "sigs.k8s.io/yaml"
)

type TemplateRenderer struct {
	Registry    *repo.Registry
	ChartRef    v1alpha1.ChartRef
	Version     string
	ReleaseName string
	Namespace   string
	KubeVersion string
	ValuesFile  string
	ValuesPatch *runtime.RawExtension
	Values      map[string]interface{}

	BucketURL string
	UID       string
	PublicURL string
	//W         io.Writer

	CRDs     []appapi.BucketFile
	Manifest *appapi.BucketFile
}

func (x *TemplateRenderer) Do() error {
	ctx := context.Background()
	bucket, err := blob.OpenBucket(ctx, x.BucketURL)
	if err != nil {
		return err
	}

	dirManifest := blob.PrefixedBucket(bucket, x.UID+"/manifests/")
	defer dirManifest.Close()
	dirCRD := blob.PrefixedBucket(bucket, x.UID+"/crds/")
	defer dirCRD.Close()

	chrt, err := x.Registry.GetChart(x.ChartRef.URL, x.ChartRef.Name, x.Version)
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
			// Open the key "${releaseName}.yaml" for writing with the default options.
			w, err := dirCRD.NewWriter(ctx, crd.Name+".yaml", nil)
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

			objectKey := "/" + path.Join(x.UID, "crds", crd.Name+".yaml")
			x.CRDs = append(x.CRDs, appapi.BucketFile{
				URL:      x.PublicURL + objectKey,
				Key:      objectKey,
				Filename: crd.Filename,
				Data:     crd.File.Data,
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
		objectKey := "/" + path.Join(x.UID, "manifests", x.ReleaseName+".yaml")
		x.Manifest = &appapi.BucketFile{
			URL:      x.PublicURL + objectKey,
			Key:      objectKey,
			Filename: "manifest.yaml",
			Data:     manifestDoc.Bytes(),
		}

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
	}

	return nil
}

func (x *TemplateRenderer) Result() (crds []appapi.BucketFile, manifest *appapi.BucketFile) {
	crds = x.CRDs
	manifest = x.Manifest

	return
}

type EditorModelGenerator struct {
	Registry    *repo.Registry
	ChartRef    v1alpha1.ChartRef
	Version     string
	ReleaseName string
	Namespace   string
	KubeVersion string
	ValuesFile  string
	ValuesPatch *runtime.RawExtension
	Values      map[string]interface{}

	RefillMetadata bool

	CRDs     []*chart.File
	Manifest []byte
}

func (x *EditorModelGenerator) Do() error {
	chrt, err := x.Registry.GetChart(x.ChartRef.URL, x.ChartRef.Name, x.Version)
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

		// opts / model needs to be updated for metadata
		if x.RefillMetadata {
			err = RefillMetadata(hub.NewRegistryOfKnownResources(), chrt.Values, vals)
			if err != nil {
				return err
			}
		}
	}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	for _, crd := range chrt.CRDObjects() {
		x.CRDs = append(x.CRDs, crd.File)
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
		x.Manifest = manifestDoc.Bytes()
	}

	return nil
}

func (x *EditorModelGenerator) Result() ([]*chart.File, []byte) {
	return x.CRDs, x.Manifest
}

func RefillMetadata(mapper discovery.ResourceMapper, ref, actual map[string]interface{}) error {
	refResources, ok := ref["resources"].(map[string]interface{})
	if !ok {
		return nil
	}
	actualResources, ok := actual["resources"].(map[string]interface{})
	if !ok {
		return nil
	}

	rlsName, _, err := unstructured.NestedString(actual, "metadata", "release", "name")
	if err != nil {
		return err
	}
	rlsNamespace, _, err := unstructured.NestedString(actual, "metadata", "release", "namespace")
	if err != nil {
		return err
	}

	for key, o := range actualResources {
		// apiVersion
		// kind
		// metadata:
		//	name:
		//  namespace:
		//  labels:

		refObj, ok := refResources[key].(map[string]interface{})
		if !ok {
			return fmt.Errorf("missing key %s in reference chart values", key)
		}
		obj := o.(map[string]interface{})
		obj["apiVersion"] = refObj["apiVersion"]
		obj["kind"] = refObj["kind"]

		// name
		name := rlsName
		idx := strings.IndexRune(key, '_')
		if idx != -1 {
			name += "-" + flect.Dasherize(key[idx+1:])
		}
		err = unstructured.SetNestedField(obj, name, "metadata", "name")
		if err != nil {
			return err
		}

		// namespace
		// TODO: add namespace if needed
		err = unstructured.SetNestedField(obj, rlsNamespace, "metadata", "namespace")
		if err != nil {
			return err
		}

		// get select labels from app and set to obj labels
		err = updateLabels(rlsName, obj, "metadata", "labels")
		if err != nil {
			return err
		}

		gvk := schema.FromAPIVersionAndKind(refObj["apiVersion"].(string), refObj["kind"].(string))
		if gvr, err := mapper.GVR(gvk); err == nil {
			if rd, err := resourceeditors.LoadByName(resourceeditors.DefaultEditorName(gvr)); err == nil {
				if rd.Spec.UI != nil {
					for _, fields := range rd.Spec.UI.InstanceLabelPaths {
						fields := strings.Trim(fields, ".")
						err = updateLabels(rlsName, obj, strings.Split(fields, ".")...)
						if err != nil {
							return err
						}
					}
				}
			}
		}

		actualResources[key] = obj
	}
	return nil
}

func updateLabels(rlsName string, obj map[string]interface{}, fields ...string) error {
	labels, ok, err := unstructured.NestedStringMap(obj, fields...)
	if err != nil {
		return err
	}
	if !ok {
		labels = map[string]string{}
	}
	key := "app.kubernetes.io/instance"
	if _, ok := labels[key]; ok {
		labels[key] = rlsName
	}
	return unstructured.SetNestedStringMap(obj, labels, fields...)
}
