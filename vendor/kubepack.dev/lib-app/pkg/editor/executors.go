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

	actionx "kubepack.dev/lib-helm/pkg/action"
	libchart "kubepack.dev/lib-helm/pkg/chart"
	"kubepack.dev/lib-helm/pkg/repo"

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
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"kmodules.xyz/resource-metadata/hub"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yamllib "sigs.k8s.io/yaml"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

type TemplateRenderer struct {
	Registry repo.IRegistry
	releasesapi.ChartSourceRef
	ReleaseName string
	Namespace   string
	KubeVersion string
	ValuesFile  string
	ValuesPatch *runtime.RawExtension
	Values      map[string]interface{}

	BucketURL string
	UID       string
	PublicURL string
	// W         io.Writer

	CRDs               []releasesapi.BucketFile
	Manifest           *releasesapi.BucketFile
	IsFeaturesetEditor bool
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
			x.CRDs = append(x.CRDs, releasesapi.BucketFile{
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
		x.Manifest = &releasesapi.BucketFile{
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

func (x *TemplateRenderer) Result() (crds []releasesapi.BucketFile, manifest *releasesapi.BucketFile) {
	crds = x.CRDs
	manifest = x.Manifest

	return
}

type EditorModelGenerator struct {
	Registry repo.IRegistry
	releasesapi.ChartSourceRef
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

func (x *EditorModelGenerator) Do(kc client.Client) error {
	chrt, err := x.Registry.GetChart(x.ChartSourceRef)
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
		klog.Warningf("WARNING: chart %+v is deprecated", x.ChartSourceRef)
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
			if data, ok := chrt.Chart.Metadata.Annotations["meta.x-helm.dev/editor"]; ok && data != "" {
				var gvr metav1.GroupVersionResource
				if err := json.Unmarshal([]byte(data), &gvr); err != nil {
					return fmt.Errorf("failed to parse %s annotation %s", "meta.x-helm.dev/editor", data)
				}
				rls := types.NamespacedName{
					Namespace: x.Namespace,
					Name:      x.ReleaseName,
				}
				if err := actionx.RefillMetadata(kc, chrt.Chart.Values, vals, gvr, rls); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("chart %+v is missing annotation key meta.x-helm.dev/editor", x.ChartSourceRef)
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
