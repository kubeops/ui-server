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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	policyapi "kubeops.dev/ui-server/apis/policy/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"

	"gomodules.xyz/sets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/dynamic"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
	dc dynamic.Interface
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client, dc dynamic.Interface) *Storage {
	return &Storage{
		kc: kc,
		dc: dc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return policyapi.SchemeGroupVersion.WithKind(policyapi.ResourceKindPolicyReport)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &policyapi.PolicyReport{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*policyapi.PolicyReport)

	resourceGraph, err := getResourceGraph(r.kc, in.Request.ObjectInfo)
	if err != nil {
		return nil, err
	}

	resp, err := r.locateResource(ctx, resourceGraph)
	if err != nil {
		return nil, err
	}

	in.Response = resp
	return in, nil
}

func (r *Storage) locateResource(ctx context.Context, resourceGraph *v1alpha1.ResourceGraphResponse) (*policyapi.PolicyReportResponse, error) {
	var resp policyapi.PolicyReportResponse

	templates, err := r.dc.Resource(schema.GroupVersionResource{
		Group:    "templates.gatekeeper.sh",
		Version:  "v1",
		Resource: "constrainttemplates",
	}).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, template := range templates.Items {
		constraintKind, _, err := unstructured.NestedString(template.UnstructuredContent(), "spec", "crd", "spec", "names", "kind")
		if err != nil {
			return nil, err
		}
		constraints, err := r.dc.Resource(schema.GroupVersionResource{
			Group:    "constraints.gatekeeper.sh",
			Version:  "v1beta1",
			Resource: strings.ToLower(constraintKind),
		}).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, constraint := range constraints.Items {
			violations, err := convertUnstructuredToViolationsArray(constraint)
			if err != nil {
				return nil, err
			}

			constraintName, auditTime, err := getNameAndAuditTime(constraint)
			if err != nil {
				return nil, err
			}

			c := policyapi.Constraint{
				AuditTimestamp: metav1.Time{Time: auditTime},
				Name:           constraintName,
				Violations:     evaluateForSingleConstraint(resourceGraph, violations),
			}
			if len(c.Violations) > 0 {
				resp.Constraints = append(resp.Constraints, c)
			}
		}
	}
	return &resp, nil
}

func getResourceGraph(kc client.Client, oi kmapi.ObjectInfo) (*v1alpha1.ResourceGraphResponse, error) {
	rid := oi.Resource
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

	return graph.ResourceGraph(kc.RESTMapper(), src)
}

func convertUnstructuredToViolationsArray(constraint unstructured.Unstructured) (policyapi.Violations, error) {
	var violations policyapi.Violations
	vs, _, err := unstructured.NestedSlice(constraint.UnstructuredContent(), "status", "violations")
	if err != nil {
		return nil, err
	}
	for _, violation := range vs {
		jsonBytes, err := json.Marshal(violation)
		if err != nil {
			return nil, err
		}
		var vv policyapi.Violation
		err = json.Unmarshal(jsonBytes, &vv)
		if err != nil {
			return nil, err
		}
		violations = append(violations, vv)
	}
	return violations, nil
}

func getNameAndAuditTime(constraint unstructured.Unstructured) (string, time.Time, error) {
	constraintName, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "metadata", "name")
	if err != nil {
		return "", time.Time{}, nil
	}

	t, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "status", "auditTimestamp")
	if err != nil {
		return "", time.Time{}, nil
	}
	auditTime, err := time.Parse(time.RFC3339, t)
	return constraintName, auditTime, err
}

/*
Complexity Analysis:
preprocess => v*lg(v) + r*lg(v) + v*lg^2(v)
buildMap   => c*lg(c)
evaluate   => v*lg(c)

where,
v = number of constraint.status.violations
r = number of resourceGraph.response.resources
c = number if resourceGraph.response.connections

So, overall n * lg^2(n) complexity for a single constraint
*/

func evaluateForSingleConstraint(gr *v1alpha1.ResourceGraphResponse, violations policyapi.Violations) policyapi.Violations {
	gvkToResourceIDMap, neededResourceIDs := preprocess(gr.Resources, violations)
	idToMeta := buildMapFromConnections(gr.Connections, neededResourceIDs)

	var toAddOnReport policyapi.Violations
	for _, violation := range violations {
		id := gvkToResourceIDMap[violationGVKToString(violation)]
		s := violationMetaToString(violation)
		if idToMeta[id].Has(s) {
			toAddOnReport = append(toAddOnReport, violation)
		}
	}
	return toAddOnReport
}

func preprocess(resources []kmapi.ResourceID, violations policyapi.Violations) (map[string]int, []int) {
	gvkToResourceIDMap := make(map[string]int)
	for _, violation := range violations {
		gvkToResourceIDMap[violationGVKToString(violation)] = -1
	}
	for id, res := range resources {
		s := resourceIDGVKToString(res)
		_, found := gvkToResourceIDMap[s]
		if found {
			gvkToResourceIDMap[s] = id
		}
	}
	neededResourceIDs := make([]int, 0)
	for s, val := range gvkToResourceIDMap {
		if val == -1 {
			delete(gvkToResourceIDMap, s)
			continue
		} else {
			neededResourceIDs = append(neededResourceIDs, val)
		}
	}
	sort.Ints(neededResourceIDs)
	return gvkToResourceIDMap, neededResourceIDs
}

func buildMapFromConnections(connections []v1alpha1.ObjectConnection, neededResourceIDs []int) []sets.String {
	mx := neededResourceIDs[len(neededResourceIDs)-1]
	idToMeta := make([]sets.String, mx+1)
	for i := range idToMeta {
		idToMeta[i] = sets.NewString()
	}

	for _, connection := range connections {
		id := connection.Source.ResourceID
		i := sort.Search(len(neededResourceIDs), func(i int) bool { return neededResourceIDs[i] == id })
		if i < len(neededResourceIDs) && neededResourceIDs[i] == id { // found
			idToMeta[id].Insert(resourceIDMetaToString(connection.Source))
		}

		id = connection.Target.ResourceID
		i = sort.Search(len(neededResourceIDs), func(i int) bool { return neededResourceIDs[i] == id })
		if i < len(neededResourceIDs) && neededResourceIDs[i] == id {
			idToMeta[id].Insert(resourceIDMetaToString(connection.Target))
		}
	}
	return idToMeta
}

func violationGVKToString(v policyapi.Violation) string {
	return fmt.Sprintf("G=%v,V=%v,K=%v", v.Group, v.Version, v.Kind)
}

func resourceIDGVKToString(r kmapi.ResourceID) string {
	return fmt.Sprintf("G=%v,V=%v,K=%v", r.Group, r.Version, r.Kind)
}

func violationMetaToString(v policyapi.Violation) string {
	return fmt.Sprintf("NS=%v,N=%v", v.Namespace, v.Name)
}

func resourceIDMetaToString(p v1alpha1.ObjectPointer) string {
	return fmt.Sprintf("NS=%v,N=%v", p.Namespace, p.Name)
}
