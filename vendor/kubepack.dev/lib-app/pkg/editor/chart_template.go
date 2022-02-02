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
	"time"

	"kubepack.dev/kubepack/apis/kubepack/v1alpha1"
	"kubepack.dev/kubepack/pkg/lib"
	appapi "kubepack.dev/lib-app/api/v1alpha1"
	"kubepack.dev/lib-helm/pkg/repo"

	"github.com/google/uuid"
	"gomodules.xyz/jsonpatch/v3"
	"helm.sh/helm/v3/pkg/chart"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kmodules.xyz/client-go/discovery"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/parser"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"sigs.k8s.io/application/api/app/v1beta1"
	app_cs "sigs.k8s.io/application/client/clientset/versioned"
	"sigs.k8s.io/yaml"
)

func RenderOrderTemplate(bs *lib.BlobStore, reg *repo.Registry, order v1alpha1.Order) (string, []appapi.ChartTemplate, error) {
	var buf bytes.Buffer
	var tpls []appapi.ChartTemplate

	for _, pkg := range order.Spec.Packages {
		if pkg.Chart == nil {
			continue
		}

		f1 := &TemplateRenderer{
			Registry:    reg,
			ChartRef:    pkg.Chart.ChartRef,
			Version:     pkg.Chart.Version,
			ReleaseName: pkg.Chart.ReleaseName,
			Namespace:   pkg.Chart.Namespace,
			KubeVersion: "v1.17.0",
			ValuesFile:  pkg.Chart.ValuesFile,
			ValuesPatch: pkg.Chart.ValuesPatch,
			BucketURL:   bs.Bucket,
			UID:         string(order.UID),
			PublicURL:   bs.Host,
		}
		err := f1.Do()
		if err != nil {
			return "", nil, err
		}

		tpl := appapi.ChartTemplate{
			ChartRef:    pkg.Chart.ChartRef,
			Version:     pkg.Chart.Version,
			ReleaseName: pkg.Chart.ReleaseName,
			Namespace:   pkg.Chart.Namespace,
		}
		crds, manifestFile := f1.Result()
		for _, crd := range crds {
			resources, err := ListResources(pkg.Chart.ReleaseName, crd.Data)
			if err != nil {
				return "", nil, err
			}
			if len(resources) != 1 {
				return "", nil, fmt.Errorf("%d crds found in %s", len(resources), crd.Filename)
			}
			tpl.CRDs = append(tpl.CRDs, appapi.BucketObject{
				URL: crd.URL,
				Key: crd.Key,
				ResourceObject: appapi.ResourceObject{
					Filename: crd.Filename,
					Data:     resources[0].Data,
				},
			})
		}
		if manifestFile != nil {
			tpl.Manifest = &appapi.BucketFileRef{
				URL: manifestFile.URL,
				Key: manifestFile.Key,
			}
			tpl.Resources, err = ListResources(pkg.Chart.ReleaseName, manifestFile.Data)
			if err != nil {
				return "", nil, err
			}
			_, err = fmt.Fprintf(&buf, "---\n# Source: %s - %s@%s\n", f1.ChartRef.URL, f1.ChartRef.Name, f1.Version)
			if err != nil {
				return "", nil, err
			}

			_, err := buf.Write(manifestFile.Data)
			if err != nil {
				return "", nil, err
			}
			_, err = buf.WriteRune('\n')
			if err != nil {
				return "", nil, err
			}
		}
		tpls = append(tpls, tpl)
	}

	return buf.String(), tpls, nil
}

func LoadEditorModel(cfg *rest.Config, reg *repo.Registry, opts appapi.ModelMetadata) (*appapi.EditorTemplate, error) {
	ed, err := resourceeditors.LoadByName(resourceeditors.DefaultEditorName(opts.Resource.GroupVersionResource()))
	if err != nil {
		return nil, err
	}

	chrt, err := reg.GetChart(ed.Spec.UI.Editor.URL, ed.Spec.UI.Editor.Name, ed.Spec.UI.Editor.Version)
	if err != nil {
		return nil, err
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	mapper := discovery.NewResourceMapper(discovery.NewRestMapper(kc.Discovery()))
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	ac, err := app_cs.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	app, err := ac.AppV1beta1().Applications(opts.Release.Namespace).Get(context.TODO(), opts.Release.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return EditorChartValueManifest(app, mapper, dc, opts.Metadata.Release, chrt.Chart)
}

func EditorChartValueManifest(app *v1beta1.Application, mapper discovery.ResourceMapper, dc dynamic.Interface, mt appapi.ObjectMeta, chrt *chart.Chart) (*appapi.EditorTemplate, error) {
	selector, err := metav1.LabelSelectorAsSelector(app.Spec.Selector)
	if err != nil {
		return nil, err
	}
	labelSelector := selector.String()

	var buf bytes.Buffer
	resourceMap := map[string]interface{}{}

	// detect apiVersion from defaultValues in chart
	gkToVersion := map[metav1.GroupKind]string{}
	for rsKey, x := range chrt.Values["resources"].(map[string]interface{}) {
		var tm metav1.TypeMeta

		err = meta_util.DecodeObject(x.(map[string]interface{}), &tm)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TypeMeta for rsKey %s in chart name=%s version=%s values", rsKey, chrt.Name(), chrt.Metadata.Version)
		}
		gv, err := schema.ParseGroupVersion(tm.APIVersion)
		if err != nil {
			return nil, err
		}
		gkToVersion[metav1.GroupKind{
			Group: gv.Group,
			Kind:  tm.Kind,
		}] = gv.Version
	}

	var resources []*unstructured.Unstructured
	for _, gk := range app.Spec.ComponentGroupKinds {
		version, ok := gkToVersion[gk]
		if !ok {
			return nil, fmt.Errorf("failed to detect version for GK %#v in chart name=%s version=%s values", gk, chrt.Name(), chrt.Metadata.Version)
		}

		gvk := schema.GroupVersionKind{
			Group:   gk.Group,
			Version: version,
			Kind:    gk.Kind,
		}
		gvr, err := mapper.GVR(gvk)
		if err != nil {
			return nil, fmt.Errorf("failed to detect GVR for gvk %v, reason %v", gvk, err)
		}
		namespaced, err := mapper.IsGVRNamespaced(gvr)
		if err != nil {
			return nil, fmt.Errorf("failed to detect if gvr %v is namespaced, reason %v", gvr, err)
		}
		var rc dynamic.ResourceInterface
		if namespaced {
			rc = dc.Resource(gvr).Namespace(mt.Namespace)
		} else {
			rc = dc.Resource(gvr)
		}

		list, err := rc.List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range list.Items {
			// remove status
			delete(obj.Object, "status")

			resources = append(resources, &obj)

			buf.WriteString("\n---\n")
			data, err := yaml.Marshal(&obj)
			if err != nil {
				return nil, err
			}
			buf.Write(data)

			rsKey, err := ResourceKey(obj.GetAPIVersion(), obj.GetKind(), mt.Name, obj.GetName())
			if err != nil {
				return nil, err
			}
			if _, ok := resourceMap[rsKey]; ok {
				return nil, fmt.Errorf("duplicate resource key %s for application %s/%s", rsKey, app.Namespace, app.Name)
			}
			resourceMap[rsKey] = &obj
		}
	}

	s1map := map[string]int{}
	s2map := map[string]int{}
	s3map := map[string]int{}
	for _, obj := range resources {
		s1, s2, s3 := ResourceFilename(obj.GetAPIVersion(), obj.GetKind(), mt.Name, obj.GetName())
		if v, ok := s1map[s1]; !ok {
			s1map[s1] = 1
		} else {
			s1map[s1] = v + 1
		}
		if v, ok := s2map[s2]; !ok {
			s2map[s2] = 1
		} else {
			s2map[s2] = v + 1
		}
		if v, ok := s3map[s3]; !ok {
			s3map[s3] = 1
		} else {
			s3map[s3] = v + 1
		}
	}

	rsfiles := make([]appapi.ResourceObject, 0, len(resources))
	for _, obj := range resources {
		s1, s2, s3 := ResourceFilename(obj.GetAPIVersion(), obj.GetKind(), mt.Name, obj.GetName())
		name := s1
		if s1map[s1] > 1 {
			if s2map[s2] > 1 {
				name = s3
			} else {
				name = s2
			}
		}
		rsfiles = append(rsfiles, appapi.ResourceObject{
			Filename: name,
			Data:     obj,
		})
	}

	tpl := appapi.EditorTemplate{
		Manifest: buf.Bytes(),
		Values: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"resource": chrt.Values["metadata"].(map[string]interface{})["resource"],
					"release":  mt,
				},
				"resources": resourceMap,
			},
		},
		Resources: rsfiles,
	}

	return &tpl, nil
}

func GenerateEditorModel(reg *repo.Registry, opts map[string]interface{}) (*unstructured.Unstructured, error) {
	var spec appapi.ModelMetadata
	err := meta_util.DecodeObject(opts, &spec)
	if err != nil {
		return nil, err
	}

	ed, err := resourceeditors.LoadByName(resourceeditors.DefaultEditorName(spec.Resource.GroupVersionResource()))
	if err != nil {
		return nil, err
	}

	f1 := &EditorModelGenerator{
		Registry: reg,
		ChartRef: v1alpha1.ChartRef{
			URL:  ed.Spec.UI.Options.URL,
			Name: ed.Spec.UI.Options.Name,
		},
		Version:     ed.Spec.UI.Options.Version,
		ReleaseName: spec.Metadata.Release.Name,
		Namespace:   spec.Metadata.Release.Namespace,
		KubeVersion: "v1.17.0",
		Values:      opts,
	}
	err = f1.Do()
	if err != nil {
		return nil, err
	}

	resoourceValues := map[string]interface{}{}
	_, manifest := f1.Result()
	err = parser.ProcessResources(manifest, func(ri parser.ResourceInfo) error {
		rsKey, err := ResourceKey(ri.Object.GetAPIVersion(), ri.Object.GetKind(), spec.Metadata.Release.Name, ri.Object.GetName())
		if err != nil {
			return err
		}

		// values
		resoourceValues[rsKey] = ri.Object
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata":  opts["metadata"],
			"resources": resoourceValues,
		},
	}, err
}

func RenderChartTemplate(reg *repo.Registry, opts map[string]interface{}) (string, *appapi.ChartTemplate, error) {
	var spec appapi.ModelMetadata
	err := meta_util.DecodeObject(opts, &spec)
	if err != nil {
		return "", nil, err
	}

	ed, err := resourceeditors.LoadByName(resourceeditors.DefaultEditorName(spec.Resource.GroupVersionResource()))
	if err != nil {
		return "", nil, err
	}

	f1 := &EditorModelGenerator{
		Registry: reg,
		ChartRef: v1alpha1.ChartRef{
			URL:  ed.Spec.UI.Editor.URL,
			Name: ed.Spec.UI.Editor.Name,
		},
		Version:        ed.Spec.UI.Editor.Version,
		ReleaseName:    spec.Release.Name,
		Namespace:      spec.Release.Namespace,
		KubeVersion:    "v1.17.0",
		Values:         opts,
		RefillMetadata: true,
	}
	err = f1.Do()
	if err != nil {
		return "", nil, err
	}

	tpl := appapi.ChartTemplate{
		ChartRef:    f1.ChartRef,
		Version:     f1.Version,
		ReleaseName: f1.ReleaseName,
		Namespace:   f1.Namespace,
	}

	crds, manifest := f1.Result()
	for _, crd := range crds {
		resources, err := parser.ListResources(crd.Data)
		if err != nil {
			return "", nil, err
		}
		if len(resources) != 1 {
			return "", nil, fmt.Errorf("%d crds found in %s", len(resources), crd.Name)
		}
		tpl.CRDs = append(tpl.CRDs, appapi.BucketObject{
			ResourceObject: appapi.ResourceObject{
				Filename: crd.Name,
				Data:     resources[0].Object,
			},
		})
	}
	if manifest != nil {
		tpl.Resources, err = ListResources(spec.Release.Name, manifest)
		if err != nil {
			return "", nil, err
		}
	}
	return string(manifest), &tpl, nil
}

func CreateChartOrder(reg *repo.Registry, opts appapi.ChartOrder) (*v1alpha1.Order, error) {
	// editor chart
	chrt, err := reg.GetChart(opts.URL, opts.Name, opts.Version)
	if err != nil {
		return nil, err
	}
	originalValues, err := json.Marshal(chrt.Values)
	if err != nil {
		return nil, err
	}

	modifiedValues, err := json.Marshal(opts.Values)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.CreatePatch(originalValues, modifiedValues)
	if err != nil {
		return nil, err
	}
	patchData, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	order := v1alpha1.Order{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
			Kind:       v1alpha1.ResourceKindOrder,
		}, ObjectMeta: metav1.ObjectMeta{
			Name:              opts.ReleaseName,
			Namespace:         opts.Namespace,
			UID:               types.UID(uuid.New().String()),
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1alpha1.OrderSpec{
			Packages: []v1alpha1.PackageSelection{
				{
					Chart: &v1alpha1.ChartSelection{
						ChartRef: v1alpha1.ChartRef{
							URL:  opts.URL,
							Name: opts.Name,
						},
						Version:     opts.Version,
						ReleaseName: opts.ReleaseName,
						Namespace:   opts.Namespace,
						Bundle:      nil,
						ValuesFile:  "values.yaml",
						ValuesPatch: &runtime.RawExtension{
							Raw: patchData,
						},
						Resources: nil,
						WaitFors:  nil,
					},
				},
			},
			KubeVersion: "",
		},
	}
	return &order, err
}
