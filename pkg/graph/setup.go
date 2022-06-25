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
	"fmt"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/pkg/errors"
	"gomodules.xyz/sets"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	kmapi "kmodules.xyz/client-go/api/v1"
	meta_util "kmodules.xyz/client-go/meta"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	ksets "kmodules.xyz/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"
)

func PollNewResourceTypes(cfg *restclient.Config) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		kc := kubernetes.NewForConfigOrDie(cfg)
		err := wait.PollImmediateUntil(60*time.Second, func() (done bool, err error) {
			rsLists, err := kc.Discovery().ServerPreferredResources()
			if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
				klog.ErrorS(err, "failed to list server preferred resources")
				return false, nil
			}
			for _, rsList := range rsLists {
				for _, rs := range rsList.APIResources {
					// skip sub resource
					if strings.ContainsRune(rs.Name, '/') {
						continue
					}

					// if resource can't be listed or read (get) skip it
					verbs := sets.NewString(rs.Verbs...)
					if !verbs.HasAll("list", "get", "watch") {
						continue
					}

					gvk := schema.FromAPIVersionAndKind(rsList.GroupVersion, rs.Kind)
					if gkSet.Has(gvk.GroupKind()) {
						continue
					}

					scope := kmapi.ClusterScoped
					if rs.Namespaced {
						scope = kmapi.NamespaceScoped
					}
					rid := kmapi.ResourceID{
						Group:   gvk.Group,
						Version: gvk.Version,
						Name:    rs.Name,
						Kind:    rs.Kind,
						Scope:   scope,
					}
					if _, found := resourceTracker[gvk]; !found {
						resourceTracker[gvk] = rid
						resourceChannel <- rid
					}
				}
			}
			return false, nil
		}, ctx.Done())
		if err != nil {
			return err
		}

		close(resourceChannel)
		return nil
	}
}

func SetupGraphReconciler(mgr manager.Manager) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		for rid := range resourceChannel {
			if err := (&Reconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
				R:      rid,
			}).SetupWithManager(mgr); err != nil {
				return err
			}
		}
		return nil
	}
}

func ExecGraphQLQuery(c client.Client, query string, vars map[string]interface{}) ([]unstructured.Unstructured, error) {
	refs, err := execRawGraphQLQuery(query, vars)
	if err != nil {
		return nil, err
	}

	var gk schema.GroupKind
	if v, ok := vars[sharedapi.GraphQueryVarTargetGroup]; ok {
		gk.Group = v.(string)
	} else {
		return nil, fmt.Errorf("vars is missing %s", sharedapi.GraphQueryVarTargetGroup)
	}
	if v, ok := vars[sharedapi.GraphQueryVarTargetKind]; ok {
		gk.Kind = v.(string)
	} else {
		return nil, fmt.Errorf("vars is missing %s", sharedapi.GraphQueryVarTargetKind)
	}

	mapping, err := c.RESTMapper().RESTMapping(gk)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect mappings for %+v", gk)
	}

	objs := make([]unstructured.Unstructured, 0, len(refs))
	for _, ref := range refs {
		var obj unstructured.Unstructured
		obj.SetGroupVersionKind(mapping.GroupVersionKind)
		err = c.Get(context.TODO(), client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &obj)
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrap(err, "failed to expand refs")
		} else if err == nil {
			objs = append(objs, obj)
		}
	}
	return objs, nil
}

func execRawGraphQLQuery(query string, vars map[string]interface{}) ([]kmapi.ObjectReference, error) {
	params := graphql.Params{
		Schema:         Schema,
		RequestString:  query,
		VariableValues: vars,
	}
	result := graphql.Do(params)
	if result.HasErrors() {
		var errs []error
		for _, e := range result.Errors {
			errs = append(errs, e)
		}
		return nil, errors.Wrap(utilerrors.NewAggregate(errs), "failed to execute graphql operation")
	}

	refs, err := listRefs(result.Data.(map[string]interface{}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract refs")
	}
	return refs, nil
}

func listRefs(data map[string]interface{}) ([]kmapi.ObjectReference, error) {
	result := ksets.NewObjectReference()
	err := extractRefs(data, result)
	return result.List(), err
}

func extractRefs(data map[string]interface{}, result ksets.ObjectReference) error {
	for k, v := range data {
		switch u := v.(type) {
		case map[string]interface{}:
			if err := extractRefs(u, result); err != nil {
				return err
			}
		case []interface{}:
			if k == "refs" {
				var refs []kmapi.ObjectReference
				err := meta_util.DecodeObject(u, &refs)
				if err != nil {
					return err
				}
				result.Insert(refs...)
				break
			}

			for i := range u {
				entry, ok := u[i].(map[string]interface{})
				if ok {
					if err := extractRefs(entry, result); err != nil {
						return err
					}
				}
			}
		default:
		}
	}
	return nil
}

func ExecRawQuery(kc client.Client, src kmapi.OID, target sharedapi.ResourceLocator) (*kmapi.ResourceID, []kmapi.ObjectReference, error) {
	mapping, err := kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: target.Ref.Group,
		Kind:  target.Ref.Kind,
	})
	if err != nil {
		return nil, nil, err
	}
	rid := kmapi.NewResourceID(mapping)

	q, vars, err := target.GraphQuery(src)
	if err != nil {
		return nil, nil, err
	}

	if target.Query.Type == sharedapi.GraphQLQuery {
		result, err := execRawGraphQLQuery(q, vars)
		return rid, result, err
	}

	obj, err := execRestQuery(kc, q, mapping.GroupVersionKind)
	if err != nil {
		return nil, nil, err
	}
	ref := kmapi.ObjectReference{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	return rid, []kmapi.ObjectReference{ref}, nil
}

func ExecQuery(kc client.Client, src kmapi.OID, target sharedapi.ResourceLocator) (*kmapi.ResourceID, []unstructured.Unstructured, error) {
	mapping, err := kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: target.Ref.Group,
		Kind:  target.Ref.Kind,
	})
	if err != nil {
		return nil, nil, err
	}
	rid := kmapi.NewResourceID(mapping)

	q, vars, err := target.GraphQuery(src)
	if err != nil {
		return nil, nil, err
	}

	if target.Query.Type == sharedapi.GraphQLQuery {
		result, err := ExecGraphQLQuery(kc, q, vars)
		return rid, result, err
	}

	obj, err := execRestQuery(kc, q, mapping.GroupVersionKind)
	if err != nil {
		return rid, nil, err
	}
	return rid, []unstructured.Unstructured{*obj}, nil
}

func execRestQuery(kc client.Client, q string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	var out unstructured.Unstructured
	err := yaml.Unmarshal([]byte(q), &out)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal query %s", q)
	}

	out.SetGroupVersionKind(gvk)
	err = kc.Create(context.TODO(), &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
