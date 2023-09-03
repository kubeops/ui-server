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

package reports

import (
	"context"

	falcoapi "kubeops.dev/falco-ui-server/apis/falco/v1alpha1"
	falcoreportsapi "kubeops.dev/falco-ui-server/apis/reports/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return falcoreportsapi.SchemeGroupVersion.WithKind(falcoreportsapi.ResourceKindFalcoReport)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &falcoreportsapi.FalcoReport{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*falcoreportsapi.FalcoReport)
	in.Response = &falcoreportsapi.FalcoReportResponse{}

	var feList falcoapi.FalcoEventList
	err := r.kc.List(ctx, &feList)
	if err != nil {
		return nil, err
	}

	if in.Request == nil || shared.IsClusterRequest(&in.Request.ObjectInfo) {
		for i := 0; i < len(feList.Items); i++ {
			fe := feList.Items[i]
			in.Response.FalcoEventRefs = append(in.Response.FalcoEventRefs, fe.Name)
		}
		return in, nil
	}
	if shared.IsNamespaceRequest(&in.Request.ObjectInfo) {
		ns := in.Request.ObjectInfo.Ref.Name
		for i := 0; i < len(feList.Items); i++ {
			fe := feList.Items[i]
			if fe.GetLabels()["k8s.ns.name"] == ns {
				in.Response.FalcoEventRefs = append(in.Response.FalcoEventRefs, fe.Name)
			}
		}
		return in, nil
	}

	resourceGraph, err := getResourceGraph(r.kc, in.Request.ObjectInfo)
	if err != nil {
		return nil, err
	}

	in.Response = r.locateResource(resourceGraph, feList)
	return in, nil
}

func getResourceGraph(kc client.Client, oi kmapi.ObjectInfo) (*v1alpha1.ResourceGraphResponse, error) {
	rid := oi.Resource
	if rid.Group == "core" {
		rid.Group = ""
	}

	if rid.Kind == "" {
		r2, err := kmapi.ExtractResourceID(kc.RESTMapper(), oi.Resource)
		if err != nil {
			return nil, err
		}
		rid = *r2
	}

	src := kmapi.ObjectID{
		Group:     rid.Group,
		Kind:      rid.Kind,
		Namespace: oi.Ref.Namespace,
		Name:      oi.Ref.Name,
	}

	return graph.ResourceGraph(kc.RESTMapper(), src, []kmapi.EdgeLabel{
		kmapi.EdgeLabelEvent,
	})
}

func (r *Storage) locateResource(resourceGraph *v1alpha1.ResourceGraphResponse, feList falcoapi.FalcoEventList) *falcoreportsapi.FalcoReportResponse {
	var resp falcoreportsapi.FalcoReportResponse
	podRefs := r.getConnectedPodRefs(resourceGraph)
	if podRefs == nil {
		return nil
	}

	for _, p := range podRefs {
		for i := 0; i < len(feList.Items); i++ {
			fe := feList.Items[i]
			podName := fe.GetLabels()["k8s.pod.name"]
			nsName := fe.GetLabels()["k8s.ns.name"]
			if p.Name == podName && p.Namespace == nsName {
				resp.FalcoEventRefs = append(resp.FalcoEventRefs, fe.Name)
			}
		}
	}
	return &resp
}

func (r *Storage) getConnectedPodRefs(g *v1alpha1.ResourceGraphResponse) []kmapi.ObjectReference {
	podID := -1
	for i := range g.Resources {
		res := g.Resources[i]
		if res.Group == "" && res.Version == "v1" && res.Kind == "Pod" {
			podID = i
			break
		}
	}
	if podID == -1 {
		return nil
	}
	mp := make(map[kmapi.ObjectReference]bool)
	for _, c := range g.Connections {
		appendIfFound := func(p v1alpha1.ObjectPointer) {
			if p.ResourceID == podID {
				mp[kmapi.ObjectReference{
					Namespace: p.Namespace,
					Name:      p.Name,
				}] = true
			}
		}
		appendIfFound(c.Source)
		appendIfFound(c.Target)
	}
	refs := make([]kmapi.ObjectReference, 0)
	for ref := range mp {
		refs = append(refs, ref)
	}
	return refs
}
