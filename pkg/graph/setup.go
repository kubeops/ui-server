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
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/pkg/errors"
	"gomodules.xyz/sets"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	apiv1 "kmodules.xyz/client-go/api/v1"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	ksets "kmodules.xyz/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

					rid := apiv1.ResourceID{
						Group:   gvk.Group,
						Version: gvk.Version,
						Name:    rs.Name,
						Kind:    rs.Kind,
						Scope:   apiv1.ClusterScoped,
					}
					if rs.Namespaced {
						rid.Scope = apiv1.NamespaceScoped
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

func ExecQuery(c client.Client, query string, vars map[string]interface{}) ([]unstructured.Unstructured, error) {
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

	objs := make([]unstructured.Unstructured, 0, len(refs))
	for _, ref := range refs {
		var obj unstructured.Unstructured
		obj.SetAPIVersion("v1")
		obj.SetKind("Pod")
		err = c.Get(context.TODO(), client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &obj)
		if err != nil {
			return nil, errors.Wrap(err, "failed to extract refs")
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func listRefs(data map[string]interface{}) ([]apiv1.ObjectReference, error) {
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
				var refs []apiv1.ObjectReference
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

func RenderSection(cfg *restclient.Config, kc client.Client, src apiv1.OID, target v1alpha1.ResourceLocator, convertToTable bool) (*v1alpha1.PageSection, error) {
	q, vars := target.GraphQuery(src)
	objs, err := ExecQuery(kc, q, vars)
	if err != nil {
		return nil, err
	}

	mapping, err := kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: target.Ref.Group,
		Kind:  target.Ref.Kind,
	})
	if err != nil {
		return nil, err
	}

	var data unstructured.UnstructuredList
	data.SetGroupVersionKind(mapping.GroupVersionKind)
	data.Items = objs

	scope := apiv1.ClusterScoped
	if mapping.Scope == meta.RESTScopeNamespace {
		scope = apiv1.NamespaceScoped
	}
	rid := apiv1.ResourceID{
		Group:   mapping.GroupVersionKind.Group,
		Version: mapping.GroupVersionKind.Version,
		Name:    mapping.Resource.Resource,
		Kind:    mapping.GroupVersionKind.Kind,
		Scope:   scope,
	}

	section := &v1alpha1.PageSection{
		Resource: rid,
	}
	if convertToTable {
		if err := Registry.Register(mapping.Resource, cfg); err != nil {
			return nil, err
		}

		table, err := tableconvertor.TableForList(Registry, kc, mapping.Resource, objs)
		if err != nil {
			return nil, err
		}
		section.Data = table
	} else {
		section.Data = &data
	}
	return section, nil
}
