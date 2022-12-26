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

package graph

import (
	"context"

	"kubeops.dev/ui-server/pkg/shared"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kmapi "kmodules.xyz/client-go/api/v1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LocatePods(ctx context.Context, kc client.Client, req *kmapi.ObjectInfo) ([]unstructured.Unstructured, error) {
	if shared.IsClusterRequest(req) || shared.IsImageRequest(req) || shared.IsClusterCVERequest(req) {
		var list unstructured.UnstructuredList
		list.SetAPIVersion("v1")
		list.SetKind("Pod")
		if err := kc.List(ctx, &list); err != nil {
			return nil, err
		}
		return list.Items, nil
	}

	rid := req.Resource

	if shared.IsNamespaceRequest(req) {
		var list unstructured.UnstructuredList
		list.SetAPIVersion("v1")
		list.SetKind("Pod")
		if err := kc.List(ctx, &list, client.InNamespace(req.Ref.Name)); err != nil {
			return nil, err
		}
		return list.Items, nil
	} else if shared.IsNamespaceCVERequest(req) {
		var list unstructured.UnstructuredList
		list.SetAPIVersion("v1")
		list.SetKind("Pod")
		if err := kc.List(ctx, &list, client.InNamespace(req.Ref.Namespace)); err != nil {
			return nil, err
		}
		return list.Items, nil
	} else if shared.IsPodRequest(req) {
		var pod unstructured.Unstructured
		pod.SetAPIVersion("v1")
		pod.SetKind("Pod")
		if err := kc.Get(ctx, req.Ref.ObjectKey(), &pod); client.IgnoreNotFound(err) != nil {
			return nil, err
		} else if err == nil {
			return []unstructured.Unstructured{pod}, nil
		}
		return nil, nil
	}

	if rid.Kind == "" {
		r2, err := kmapi.ExtractResourceID(kc.RESTMapper(), req.Resource)
		if err != nil {
			return nil, err
		}
		rid = *r2
	}

	src := kmapi.ObjectID{
		Group:     rid.Group,
		Kind:      rid.Kind,
		Namespace: req.Ref.Namespace,
		Name:      req.Ref.Name,
	}
	target := sharedapi.ResourceLocator{
		Ref: metav1.GroupKind{
			Group: "",
			Kind:  "Pod",
		},
		Query: sharedapi.ResourceQuery{
			Type:    sharedapi.GraphQLQuery,
			ByLabel: kmapi.EdgeOffshoot,
		},
	}

	_, refs, err := ExecRawQuery(kc, src.OID(), target)
	if err != nil {
		return nil, err
	}

	pods := make([]unstructured.Unstructured, 0, len(refs))
	for _, ref := range refs {
		var pod unstructured.Unstructured
		pod.SetAPIVersion("v1")
		pod.SetKind("Pod")
		if err := kc.Get(ctx, ref.ObjectKey(), &pod); client.IgnoreNotFound(err) != nil {
			return nil, err
		} else if err == nil {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}
