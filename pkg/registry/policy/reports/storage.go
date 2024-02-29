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
	"kubeops.dev/ui-server/pkg/shared"

	"github.com/open-policy-agent/gatekeeper/v3/pkg/audit"
	"gomodules.xyz/sets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return policyapi.SchemeGroupVersion.WithKind(policyapi.ResourceKindPolicyReport)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(policyapi.ResourceKindPolicyReport)
}

func (r *Storage) New() runtime.Object {
	return &policyapi.PolicyReport{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*policyapi.PolicyReport)

	var (
		scp           scopeDetails
		resourceGraph *v1alpha1.ResourceGraphResponse
		err           error
	)
	if in.Request == nil || shared.IsClusterRequest(&in.Request.ObjectInfo) {
		scp.isCluster = true
	} else if shared.IsNamespaceRequest(&in.Request.ObjectInfo) {
		scp.isNamespace = true
		scp.namespace = in.Request.ObjectInfo.Ref.Name
	} else {
		resourceGraph, err = getResourceGraph(r.kc, in.Request.ObjectInfo)
		if err != nil {
			return nil, err
		}
	}

	resp, err := r.locateResource(ctx, resourceGraph, scp)
	if err != nil {
		return nil, err
	}

	in.Response = resp

	// resp sorted by constraints' names.
	sort.Slice(resp.Constraints, func(i, j int) bool {
		return in.Response.Constraints[i].Name < in.Response.Constraints[j].Name
	})
	return in, nil
}

type scopeDetails struct {
	isCluster   bool
	isNamespace bool
	namespace   string
}

func (r *Storage) locateResource(ctx context.Context, resourceGraph *v1alpha1.ResourceGraphResponse, scp scopeDetails) (*policyapi.PolicyReportResponse, error) {
	var resp policyapi.PolicyReportResponse
	templates, err := ListTemplates(ctx, r.kc)
	if err != nil {
		return nil, err
	}

	for _, template := range templates.Items {
		constraintKind, _, err := unstructured.NestedString(template.UnstructuredContent(), "spec", "crd", "spec", "names", "kind")
		if err != nil {
			return nil, err
		}
		constraints, err := ListConstraints(ctx, r.kc, constraintKind)
		if err != nil {
			return nil, err
		}
		for _, constraint := range constraints.Items {
			violations, err := GetViolationsOfConstraint(constraint)
			if err != nil {
				return nil, err
			}

			constraintName, err := GetNameOfConstraint(constraint)
			if err != nil {
				return nil, err
			}
			auditTime, err := GetAuditTimeOfConstraint(constraint)
			if err != nil {
				return nil, err
			}
			resource, err := GetResourceFQNOfConstraint(constraint)
			if err != nil {
				return nil, err
			}

			c := policyapi.Constraint{
				AuditTimestamp: metav1.Time{Time: auditTime},
				Name:           constraintName,
				GVR:            resource,
				Violations:     evaluateForSingleConstraint(resourceGraph, violations, scp),
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
		kmapi.EdgeLabelConfig,
		kmapi.EdgeLabelExposedBy,
		kmapi.EdgeLabelAuthn,
	})
}

func ListTemplates(ctx context.Context, kc client.Client) (unstructured.UnstructuredList, error) {
	var templates unstructured.UnstructuredList
	templates.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "templates.gatekeeper.sh",
		Version: "v1",
		Kind:    "ConstraintTemplate",
	})
	err := kc.List(ctx, &templates)
	return templates, err
}

func ListConstraints(ctx context.Context, kc client.Client, kind string) (unstructured.UnstructuredList, error) {
	var constraints unstructured.UnstructuredList
	constraints.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "constraints.gatekeeper.sh",
		Version: "v1beta1",
		Kind:    kind,
	})

	err := kc.List(ctx, &constraints)
	return constraints, err
}

func GetViolationsOfConstraint(constraint unstructured.Unstructured) ([]audit.StatusViolation, error) {
	var violations []audit.StatusViolation
	vs, _, err := unstructured.NestedSlice(constraint.UnstructuredContent(), "status", "violations")
	if err != nil {
		return nil, err
	}
	for _, violation := range vs {
		jsonBytes, err := json.Marshal(violation)
		if err != nil {
			return nil, err
		}

		var vv audit.StatusViolation
		err = json.Unmarshal(jsonBytes, &vv)
		if err != nil {
			return nil, err
		}
		violations = append(violations, vv)
	}
	return violations, nil
}

func GetNameOfConstraint(constraint unstructured.Unstructured) (string, error) {
	constraintName, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "metadata", "name")
	if err != nil {
		return "", nil
	}
	return constraintName, err
}

func GetAuditTimeOfConstraint(constraint unstructured.Unstructured) (time.Time, error) {
	t, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "status", "auditTimestamp")
	if err != nil {
		return time.Time{}, nil
	}
	auditTime, err := time.Parse(time.RFC3339, t)
	return auditTime, err
}

func GetResourceFQNOfConstraint(constraint unstructured.Unstructured) (schema.GroupVersionResource, error) {
	apiVersion, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "apiVersion")
	if err != nil {
		return schema.GroupVersionResource{}, nil
	}

	kind, _, err := unstructured.NestedString(constraint.UnstructuredContent(), "kind")
	if err != nil {
		return schema.GroupVersionResource{}, nil
	}

	s := strings.Split(apiVersion, "/")
	if len(s) != 2 {
		return schema.GroupVersionResource{}, fmt.Errorf("apiVersion %s is bad structured", apiVersion)
	}
	return schema.GroupVersionResource{
		Group:    s[0],
		Version:  s[1],
		Resource: strings.ToLower(kind),
	}, err
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

func evaluateForSingleConstraint(gr *v1alpha1.ResourceGraphResponse, violations []audit.StatusViolation, scp scopeDetails) []audit.StatusViolation {
	if scp.isCluster {
		return violations
	} else if scp.isNamespace {
		return evaluateForSingleConstraintInNamespaceScope(violations, scp.namespace)
	}

	gvkToResourceIDMap, neededResourceIDs := preprocess(gr.Resources, violations)
	if len(neededResourceIDs) == 0 {
		return nil
	}
	idToMeta := buildMapFromConnections(gr.Connections, neededResourceIDs)

	var toAddOnReport []audit.StatusViolation
	for _, violation := range violations {
		id := gvkToResourceIDMap[violationGVKToString(violation)]
		s := violationMetaToString(violation)
		if idToMeta[id].Has(s) {
			toAddOnReport = append(toAddOnReport, violation)
		}
	}
	return toAddOnReport
}

func evaluateForSingleConstraintInNamespaceScope(violations []audit.StatusViolation, ns string) []audit.StatusViolation {
	ret := make([]audit.StatusViolation, 0)
	for _, v := range violations {
		if v.Namespace == ns {
			ret = append(ret, v)
		}
	}
	return ret
}

func preprocess(resources []kmapi.ResourceID, violations []audit.StatusViolation) (map[string]int, []int) {
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

func violationGVKToString(v audit.StatusViolation) string {
	return fmt.Sprintf("G=%v,V=%v,K=%v", v.Group, v.Version, v.Kind)
}

func resourceIDGVKToString(r kmapi.ResourceID) string {
	return fmt.Sprintf("G=%v,V=%v,K=%v", r.Group, r.Version, r.Kind)
}

func violationMetaToString(v audit.StatusViolation) string {
	return fmt.Sprintf("NS=%v,N=%v", v.Namespace, v.Name)
}

func resourceIDMetaToString(p v1alpha1.ObjectPointer) string {
	return fmt.Sprintf("NS=%v,N=%v", p.Namespace, p.Name)
}
