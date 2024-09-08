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

package graph

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jmespath/go-jmespath"
	"gomodules.xyz/jsonpath"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/pointer"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	ksets "kmodules.xyz/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func appendObjects(arr []*unstructured.Unstructured, items ...*unstructured.Unstructured) []*unstructured.Unstructured {
	m := make(map[types.NamespacedName]*unstructured.Unstructured)

	for i := range arr {
		m[types.NamespacedName{Namespace: arr[i].GetNamespace(), Name: arr[i].GetName()}] = arr[i]
	}
	for i := range items {
		m[types.NamespacedName{Namespace: items[i].GetNamespace(), Name: items[i].GetName()}] = items[i]
	}

	out := make([]*unstructured.Unstructured, 0, len(m))
	for _, obj := range m {
		out = append(out, obj)
	}
	return out
}

type ObjectFinder struct {
	Client client.Client
}

func (finder ObjectFinder) List(src *unstructured.Unstructured, path []*Edge) ([]*unstructured.Unstructured, error) {
	in := []*unstructured.Unstructured{src}
	if len(path) == 0 {
		return in, nil
	}

	var out []*unstructured.Unstructured
	for _, e := range path {
		out = nil
		for _, inObj := range in {
			result, err := finder.ResourcesFor(inObj, e)
			if err != nil && !kerr.IsNotFound(err) {
				return nil, err
			}
			out = appendObjects(out, result...)
		}
		in = out
	}

	return out, nil
}

func (finder ObjectFinder) ListConnectedResources(src *unstructured.Unstructured, edges AdjacencyMap) (map[schema.GroupVersionKind][]*unstructured.Unstructured, error) {
	result := make(map[schema.GroupVersionKind][]*unstructured.Unstructured)

	for dstGVR, e := range edges {
		objects, err := finder.ResourcesFor(src, e)
		if kerr.IsNotFound(err) || meta.IsNoMatchError(err) || (err == nil && len(objects) == 0) {
			continue
		} else if err != nil {
			return nil, err
		}
		result[dstGVR] = objects
	}

	return result, nil
}

func (finder ObjectFinder) ListConnectedPartials(src *unstructured.Unstructured, edges AdjacencyMap) (map[schema.GroupVersionKind][]*metav1.PartialObjectMetadata, error) {
	result := make(map[schema.GroupVersionKind][]*metav1.PartialObjectMetadata)

	for dstGVR, e := range edges {
		objects, err := finder.ResourcesFor(src, e)
		if kerr.IsNotFound(err) || meta.IsNoMatchError(err) || (err == nil && len(objects) == 0) {
			continue
		} else if err != nil {
			return nil, err
		}
		partials := make([]*metav1.PartialObjectMetadata, 0, len(objects))
		for _, obj := range objects {
			var pt metav1.PartialObjectMetadata
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &pt); err != nil {
				return nil, err
			} else {
				partials = append(partials, &pt)
			}
		}
		result[dstGVR] = partials
	}

	return result, nil
}

func (finder ObjectFinder) ListConnectedObjectIDs(src *unstructured.Unstructured, connections []rsapi.ResourceConnection) (map[kmapi.EdgeLabel]ksets.OID, error) {
	type GKL struct {
		Group  string
		Kind   string
		Labels string
	}
	srcGVK := src.GroupVersionKind()
	connsPerGKL := map[GKL][]rsapi.ResourceConnection{}
	for _, c := range connections {
		gvk := c.Target.GroupVersionKind()
		labels := make([]string, 0, len(c.Labels))
		for _, lbl := range c.Labels {
			labels = append(labels, string(lbl))
		}
		sort.Strings(labels)
		gkl := GKL{
			Group:  gvk.Group,
			Kind:   gvk.Kind,
			Labels: strings.Join(labels, ","),
		}
		connsPerGKL[gkl] = append(connsPerGKL[gkl], c)
	}

	edges := map[kmapi.EdgeLabel]ksets.OID{}
	for _, conns := range connsPerGKL {
		if len(conns) > 1 {
			sort.Slice(conns, func(i, j int) bool {
				d, _ := apiversion.Compare(conns[i].Target.GroupVersionKind().Version, conns[j].Target.GroupVersionKind().Version)
				return d > 0
			})
		}
		objects, err := finder.ResourcesFor(src, &Edge{
			Src:        srcGVK,
			Dst:        conns[0].Target.GroupVersionKind(),
			W:          0,
			Connection: conns[0].ResourceConnectionSpec,
			Forward:    true,
		})
		if kerr.IsNotFound(err) || meta.IsNoMatchError(err) || (err == nil && len(objects) == 0) {
			continue
		} else if err != nil {
			return nil, err
		}
		for _, obj := range objects {
			oid := kmapi.NewObjectID(obj).OID()
			for _, lbl := range conns[0].Labels {
				if _, ok := edges[lbl]; !ok {
					edges[lbl] = ksets.NewOID()
				}
				edges[lbl].Insert(oid)
			}
		}
	}

	return edges, nil
}

func (finder ObjectFinder) ResourcesFor(src *unstructured.Unstructured, e *Edge) ([]*unstructured.Unstructured, error) {
	if e.Src != src.GroupVersionKind() {
		return nil, fmt.Errorf("edge src %v does not match ref %v", e.Src, src.GroupVersionKind())
	}

	if e.Forward {
		// FIXME: How to handle namespace for Backward direction
		if e.Connection.Type == rsapi.MatchSelector {
			// var ls string
			var selector labels.Selector
			var err error

			if e.Connection.SelectorPath != "" {
				selector, err = ExtractSelector(src, e.Connection.SelectorPath)
				if err != nil {
					return nil, err
				}
			} else if e.Connection.Selector != nil {
				s2, err := evalLabelSelector(src, e.Connection.Selector)
				if err != nil {
					return nil, err
				}
				selector, err = metav1.LabelSelectorAsSelector(s2)
				if err != nil {
					return nil, err
				}
				// ls = selector.String()
			} else {
				return nil, fmt.Errorf("edge %v is missing selectorPath and selector", e)
			}

			namespaces, err := Namespaces(finder.Client, src, e.Connection.Namespace)
			if err != nil {
				return nil, err
			}

			var out []*unstructured.Unstructured

			if _, selectable := selector.Requirements(); selectable {
				for _, ns := range namespaces {
					opts := client.ListOptions{LabelSelector: labels.Everything()}
					if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
						return nil, err
					} else if namespaced {
						opts.Namespace = ns
					}

					selInApp := e.Connection.TargetLabelPath != "" &&
						strings.Trim(e.Connection.TargetLabelPath, ".") != MetadataLabels
					if !selInApp {
						// TODO(tamal): check for correctness
						opts.LabelSelector = selector
					}
					var result unstructured.UnstructuredList
					result.SetGroupVersionKind(e.Dst) // KB: ok?
					err := finder.Client.List(context.TODO(), &result, &opts)
					if err != nil {
						return nil, err
					}
					for i := range result.Items {
						rs := result.Items[i]

						if selInApp {
							lbl, ok, err := unstructured.NestedStringMap(rs.Object, fields(e.Connection.TargetLabelPath)...)
							if err != nil {
								return nil, err
							}
							if !ok || !selector.Matches(labels.Set(lbl)) {
								continue
							}
						}

						if isConnected(e.Connection.Level, &rs, src) {
							out = append(out, &rs)
						}
					}
				}
			}
			return out, nil
		} else if e.Connection.Type == rsapi.MatchName {
			if e.Connection.NameTemplate == "" {
				return nil, fmt.Errorf("edge %v is missing nameTemplate", e)
			}
			name := strings.ReplaceAll(e.Connection.NameTemplate, MetadataNameQuery, src.GetName())

			namespaces, err := Namespaces(finder.Client, src, e.Connection.Namespace)
			if err != nil {
				return nil, err
			}

			var out []*unstructured.Unstructured
			for _, ns := range namespaces {
				objkey := client.ObjectKey{Name: name}
				if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
					return nil, err
				} else if namespaced {
					objkey.Namespace = ns
				}

				var rs unstructured.Unstructured
				rs.SetGroupVersionKind(e.Dst)
				err := finder.Client.Get(context.TODO(), objkey, &rs)
				if err != nil {
					return nil, err
				}

				if isConnected(e.Connection.Level, &rs, src) {
					out = append(out, &rs)
				}
			}
			return out, nil
		} else if e.Connection.Type == rsapi.OwnedBy {
			return finder.findOwners(e, src.GetOwnerReferences(), src.GetNamespace())
		} else if e.Connection.Type == rsapi.MatchRef {
			// TODO: check that namespacePath must be empty

			var out []*unstructured.Unstructured
			for _, reference := range e.Connection.References {
				var records [][]string
				var err error
				if strings.HasPrefix(reference, "jmes:") {
					records, err = execJmesPath(src.Object, reference[5:], e.Src, e.Dst)
					if err != nil {
						return nil, err
					}
				} else {
					records, err = execJsonPath(src.Object, reference, e.Src, e.Dst)
					if err != nil {
						return nil, err
					}
				}
				refs, err := ParseResourceRefs(records)
				if err != nil {
					return nil, err
				}

				var objects []*unstructured.Unstructured
				for _, ref := range refs {
					// if apiGroup is set, it must match
					if ref.APIGroup != "" && ref.APIGroup != e.Dst.Group {
						continue
					}
					// if apiGroup is set, it must match
					if ref.Kind != "" && ref.Kind != e.Dst.Kind {
						continue
					}

					objkey := client.ObjectKey{Name: ref.Name}
					if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
						return nil, err
					} else if namespaced {
						ns := ref.Namespace
						if ns == "" {
							ns = src.GetNamespace()
						}
						if ns == "" {
							// dst is namespaced &&
							// no namespace is defined in reference &&
							// src is not-namespaced
							return nil, errors.New("namespace must be defined in reference")
						}
						objkey.Namespace = ns
					}

					var rs unstructured.Unstructured
					rs.SetGroupVersionKind(e.Dst)
					err := finder.Client.Get(context.TODO(), objkey, &rs)
					if kerr.IsNotFound(err) {
						continue // ignore optional configMapRef, secretRef
					} else if err != nil {
						return nil, err
					}

					if isConnected(e.Connection.Level, &rs, src) {
						objects = append(objects, &rs)
					}
				}
				out = appendObjects(out, objects...)
			}
			return out, nil
		}
	} else {
		namespace := core.NamespaceAll
		if e.Connection.Namespace != nil && e.Connection.Namespace.Path == MetadataNamespace {
			namespace = src.GetNamespace()
		} // else all namespace RETHINK

		if e.Connection.Type == rsapi.MatchSelector {
			var out []*unstructured.Unstructured

			lbl := src.GetLabels()
			if e.Connection.TargetLabelPath != "" && strings.Trim(e.Connection.TargetLabelPath, ".") != MetadataLabels {
				l2, ok, err := unstructured.NestedStringMap(src.Object, fields(e.Connection.TargetLabelPath)...)
				if err != nil {
					return nil, err
				}
				if !ok {
					return out, nil // empty result
				}
				lbl = l2
			}

			opts := client.ListOptions{LabelSelector: labels.Everything()}
			if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
				return nil, err
			} else if namespaced {
				opts.Namespace = namespace
			}

			var result unstructured.UnstructuredList
			result.SetGroupVersionKind(e.Dst)
			err := finder.Client.List(context.TODO(), &result, &opts)
			if err != nil {
				return nil, err
			}
			for i := range result.Items {
				rs := result.Items[i]

				if e.Connection.Namespace != nil && e.Connection.Namespace.Path != MetadataNamespace {
					namespaces, err := Namespaces(finder.Client, &rs, e.Connection.Namespace)
					if err != nil {
						return nil, err
					}
					found := contains(namespaces, src.GetNamespace()) ||
						(len(namespaces) == 1 && namespaces[0] == core.NamespaceAll)
					if !found {
						continue
					}
				}

				var selector labels.Selector
				if e.Connection.SelectorPath != "" {
					selector, err = ExtractSelector(&rs, e.Connection.SelectorPath)
					if err != nil {
						return nil, err
					}
				} else if e.Connection.Selector != nil {
					s2, err := evalLabelSelector(&rs, e.Connection.Selector)
					if err != nil {
						return nil, err
					}
					selector, err = metav1.LabelSelectorAsSelector(s2)
					if err != nil {
						return nil, err
					}
					_ = selector.String()
				} else {
					return nil, fmt.Errorf("edge %v is missing selectorPath and selector", e)
				}

				if _, selectable := selector.Requirements(); !selectable {
					continue
				}

				if selector.Matches(labels.Set(lbl)) {
					if isConnected(e.Connection.Level, src, &rs) {
						out = append(out, &rs)
					}
				}
			}
			return out, nil
		} else if e.Connection.Type == rsapi.MatchName {
			if e.Connection.NameTemplate != "" {
				name, ok := ExtractName(src.GetName(), e.Connection.NameTemplate)
				if !ok {
					return nil, fmt.Errorf("failed to detect name from %s and %s", src.GetName(), e.Connection.NameTemplate)
				}

				var out []*unstructured.Unstructured
				objkey := client.ObjectKey{Name: name}
				if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
					return nil, err
				} else if namespaced {
					objkey.Namespace = namespace
				}
				var rs unstructured.Unstructured
				rs.SetGroupVersionKind(e.Dst)
				err := finder.Client.Get(context.TODO(), objkey, &rs)
				if err != nil {
					return nil, err
				}

				if isConnected(e.Connection.Level, src, &rs) {
					out = append(out, &rs)
				}

				return out, nil
			}
		} else if e.Connection.Type == rsapi.OwnedBy {
			return finder.findChildren(e, src)
		} else if e.Connection.Type == rsapi.MatchRef {
			// TODO: check that namespacePath must be empty

			opts := client.ListOptions{LabelSelector: labels.Everything()}
			if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
				return nil, err
			} else if namespaced {
				ns := core.NamespaceAll
				if e.Connection.Namespace != nil && e.Connection.Namespace.Path == MetadataNamespace {
					ns = src.GetNamespace()
				}
				opts.Namespace = ns
			}
			var result unstructured.UnstructuredList
			result.SetGroupVersionKind(e.Dst)
			err := finder.Client.List(context.TODO(), &result, &opts)
			if err != nil {
				return nil, err
			}

			var out []*unstructured.Unstructured
		NextItem:
			for i := range result.Items {
				rs := result.Items[i]

				for _, reference := range e.Connection.References {
					var records [][]string
					var err error
					if strings.HasPrefix(reference, "jmes:") {
						records, err = execJmesPath(rs.Object, reference[5:], e.Src, e.Dst)
						if err != nil {
							return nil, err
						}
					} else {
						records, err = execJsonPath(rs.Object, reference, e.Src, e.Dst)
						if err != nil {
							return nil, err
						}
					}
					refs, err := ParseResourceRefs(records)
					if err != nil {
						return nil, err
					}
					for _, ref := range refs {
						// if apiGroup is set, it must match
						if ref.APIGroup != "" && ref.APIGroup != e.Src.Group {
							continue
						}

						// if apiGroup is set, it must match
						if ref.Kind != "" && ref.Kind != e.Src.Kind {
							continue
						}

						ns := ref.Namespace
						namespaced, err := finder.isNamespaced(e.Src)
						if err != nil {
							return nil, err
						}
						if ns == "" && namespaced {
							ns = rs.GetNamespace()
							if ns == "" {
								// src is namespaced &&
								// no namespace is defined in reference &&
								// rs is not-namespaced
								return nil, errors.New("namespace must be defined in reference")
							}

							if ns != src.GetNamespace() {
								continue
							}
						}

						if ref.Name != src.GetName() {
							continue
						}

						if isConnected(e.Connection.Level, src, &rs) {
							out = append(out, &rs)
						}
						continue NextItem
					}
				}
			}
			return out, nil
		}
	}

	return nil, nil
}

func execJmesPath(data any, reference string, src, dst schema.GroupVersionKind) ([][]string, error) {
	result, err := jmespath.Search(reference, data)
	if err != nil {
		return nil, fmt.Errorf("fails to execute jmes reference %q between %s -> %s. err:%w", reference, src, dst, err)
	}
	switch v := result.(type) {
	case string:
		return [][]string{{v}}, nil
	case []string:
		out := make([][]string, len(v))
		for i, s := range v {
			out[i] = []string{s}
		}
		return out, nil
	case [][]string:
		return v, nil
	}
	return nil, fmt.Errorf("invalid result type %T for jmes reference %q between %s -> %s", result, reference, src, dst)
}

func execJsonPath(data any, reference string, src, dst schema.GroupVersionKind) ([][]string, error) {
	j := jsonpath.New("jsonpath")
	j.AllowMissingKeys(true)
	err := j.Parse(reference)
	if err != nil {
		return nil, fmt.Errorf("fails to parse reference %q between %s -> %s. err:%v", reference, src, dst, err)
	}
	buf := new(bytes.Buffer)
	err = j.Execute(buf, data)
	if err != nil {
		return nil, fmt.Errorf("fails to execute reference %q between %s -> %s. err:%v", reference, src, dst, err)
	}
	r := csv.NewReader(buf)
	// Mapper.Comma = ';'
	r.Comment = '#'
	return r.ReadAll()
}

func isConnected(conn rsapi.OwnershipLevel, obj *unstructured.Unstructured, owner *unstructured.Unstructured) bool {
	switch conn {
	case rsapi.Controller:
		if metav1.IsControlledBy(obj, owner) {
			return true
		}
	case rsapi.Owner:
		if IsOwnedBy(obj, owner) {
			return true
		}
	default:
		return true
	}
	return false
}

func evalLabelSelector(obj *unstructured.Unstructured, in *metav1.LabelSelector) (*metav1.LabelSelector, error) {
	out := in.DeepCopy()
	for k, v := range out.MatchLabels {
		if strings.ContainsRune(k, '{') {
			return nil, fmt.Errorf("invalid selector key %v", k)
		}
		if v == MetadataNameQuery {
			out.MatchLabels[k] = obj.GetName()
			continue
		}
		if v == MetadataNamespaceQuery {
			out.MatchLabels[k] = obj.GetNamespace()
			continue
		}
		if v[0] == '{' && v[len(v)-1] == '}' {
			val, err := evalJsonPath(obj, v)
			if err != nil {
				return nil, err
			}
			out.MatchLabels[k] = val
		}
	}
	for i := range out.MatchExpressions {
		expr := out.MatchExpressions[i]
		if strings.ContainsRune(expr.Key, '{') {
			return nil, fmt.Errorf("selector has invalid key %v", expr.Key)
		}
		for vi := range expr.Values {
			v := expr.Values[vi]
			if v[0] == '{' && v[len(v)-1] == '}' {
				val, err := evalJsonPath(obj, v)
				if err != nil {
					return nil, err
				}
				expr.Values[vi] = val
			}
		}
		out.MatchExpressions[i] = expr
	}
	return out, nil
}

func evalJsonPath(src *unstructured.Unstructured, template string) (string, error) {
	j := jsonpath.New("jsonpath")
	j.AllowMissingKeys(true)
	err := j.Parse(template)
	if err != nil {
		return "", fmt.Errorf("fails to parse value of selector key. err:%v", err)
	}
	buf := new(bytes.Buffer)
	err = j.Execute(buf, src.Object)
	if err != nil {
		return "", fmt.Errorf("fails to evaluate value of selector key. err:%v", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func (finder ObjectFinder) findOwners(e *Edge, srcOwnerRefs []metav1.OwnerReference, namespace string) ([]*unstructured.Unstructured, error) {
	var out []*unstructured.Unstructured

	objkey := client.ObjectKey{}
	if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
		return nil, err
	} else if namespaced {
		objkey.Namespace = namespace
	}
	for _, ref := range srcOwnerRefs {
		objkey.Name = ref.Name

		if ref.APIVersion == e.Dst.GroupVersion().String() && ref.Kind == e.Dst.Kind {
			if e.Connection.Level == rsapi.Controller {
				if ref.Controller != nil && *ref.Controller {
					var rs unstructured.Unstructured
					rs.SetGroupVersionKind(e.Dst)
					err := finder.Client.Get(context.TODO(), objkey, &rs)
					if err != nil {
						return nil, err
					}
					out = append(out, &rs)
					break
				}
			} else if e.Connection.Level == rsapi.Owner {
				var rs unstructured.Unstructured
				rs.SetGroupVersionKind(e.Dst)
				err := finder.Client.Get(context.TODO(), objkey, &rs)
				if err != nil {
					return nil, err
				}
				out = append(out, &rs)
			} else {
				return nil, fmt.Errorf("connection level should be Owner or Controller, found %v", e.Connection.Level)
			}
		}
	}

	return out, nil
}

func (finder ObjectFinder) findChildren(e *Edge, src *unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	if e.Connection.Level != rsapi.Owner && e.Connection.Level != rsapi.Controller {
		return nil, fmt.Errorf("connection level should be Owner or Controller, found %v", e.Connection.Level)
	}

	var out []*unstructured.Unstructured

	opts := client.ListOptions{LabelSelector: labels.Everything()}
	if namespaced, err := finder.isNamespaced(e.Dst); err != nil {
		return nil, err
	} else if namespaced {
		opts.Namespace = src.GetNamespace()
	}

	var result unstructured.UnstructuredList
	result.SetGroupVersionKind(e.Dst)
	err := finder.Client.List(context.TODO(), &result, &opts)
	if err != nil {
		return nil, err
	}
	for i := range result.Items {
		rs := result.Items[i]
		if isConnected(e.Connection.Level, &rs, src) {
			out = append(out, &rs)
		}
	}

	return out, nil
}

func (finder ObjectFinder) isNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	var versions []string
	if gvk.Version != "" {
		versions = append(versions, gvk.Version)
	}
	mapping, err := finder.Client.RESTMapper().RESTMapping(gvk.GroupKind(), versions...)
	if err != nil {
		return false, err
	}
	return mapping.Scope == meta.RESTScopeNamespace, nil
}

func IsOwnedBy(obj metav1.Object, owner metav1.Object) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.GetUID() {
			return true
		}
	}
	return false
}

// len([]string) == 0 && err == nil => all namespaces
func Namespaces(kc client.Client, ref *unstructured.Unstructured, ns *rsapi.NamespaceRef) ([]string, error) {
	if ns == nil {
		return []string{metav1.NamespaceAll}, nil
	}

	if ns.Path == MetadataNamespace {
		return []string{ref.GetNamespace()}, nil
	} else if ns.Path != "" {
		v, ok, err := unstructured.NestedString(ref.UnstructuredContent(), fields(ns.Path)...)
		if ok {
			return []string{v}, nil
		}
		return []string{}, err
	} else if ns.LabelSelector != "" {
		sel, err := ExtractSelector(ref, ns.LabelSelector)
		if err != nil {
			return nil, err
		} else if sel.Empty() {
			return []string{metav1.NamespaceAll}, nil
		} else {
			var namespaces unstructured.UnstructuredList
			namespaces.SetAPIVersion("v1")
			namespaces.SetKind("Namespace")
			err = kc.List(context.TODO(), &namespaces, client.MatchingLabelsSelector{Selector: sel})
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(namespaces.Items))
			err = namespaces.EachListItem(func(object runtime.Object) error {
				names = append(names, object.(client.Object).GetName())
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	} else if ns.Selector != "" {
		var nsel NamespaceSelector
		ok, err := Extract(ref, ns.Selector, &nsel)
		if err != nil {
			return nil, err
		}
		if ok {
			// https://github.com/coreos/prometheus-operator/blob/cc584ecfa08d2eb95ba9401f116e3a20bf71be8b/pkg/prometheus/promcfg.go#L392
			if !nsel.Any && len(nsel.MatchNames) == 0 {
				return []string{ref.GetNamespace()}, nil
			} else if len(nsel.MatchNames) > 0 {
				return nsel.MatchNames, nil
			}
			return nil, nil
		}
	}
	return []string{metav1.NamespaceAll}, nil
}

func Extract(u *unstructured.Unstructured, fieldPath string, v interface{}) (bool, error) {
	if fieldPath == "" {
		return false, errors.New("fieldPath can't be empty")
	}
	f, ok, err := unstructured.NestedMap(u.Object, fields(fieldPath)...)
	if !ok || err != nil {
		return false, err
	}
	err = meta_util.DecodeObject(f, v)
	return err == nil, err
}

func keyExists(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}

func ExtractSelector(u *unstructured.Unstructured, fieldPath string) (labels.Selector, error) {
	if fieldPath == "" {
		return labels.Nothing(), errors.New("fieldPath can't be empty")
	}
	val, found, err := unstructured.NestedFieldNoCopy(u.Object, fields(fieldPath)...)
	if !found || err != nil {
		return labels.Nothing(), err
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return labels.Nothing(), fmt.Errorf("%v accessor error: %v is of the type %T, expected map[string]interface{}", fieldPath, val, val)
	}

	if len(m) <= 2 && (keyExists(m, "matchLabels") || keyExists(m, "matchExpressions")) {
		var ls metav1.LabelSelector
		err = meta_util.DecodeObject(m, &ls)
		if err != nil {
			return labels.Nothing(), err
		}

		sel, err := metav1.LabelSelectorAsSelector(&ls)
		if err != nil {
			return labels.Nothing(), err
		}
		return sel, nil
	}

	strMap := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			strMap[k] = str
		} else {
			return labels.Nothing(), fmt.Errorf("%v accessor error: contains non-string key in the map: %v is of the type %T, expected string", fieldPath, v, v)
		}
	}
	sel := labels.SelectorFromSet(strMap)
	return sel, nil
}

func ExtractName(name, selector string) (string, bool) {
	re := regexp.MustCompile(`^` + strings.ReplaceAll(selector, MetadataNameQuery, `(.*)`) + `$`)
	matches := re.FindStringSubmatch(name)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func ParseResourceRefs(records [][]string) ([]ResourceRef, error) {
	var refs []ResourceRef

	var cols int
NEXT:
	for i, rec := range records {
		n := len(rec)
		if i == 0 {
			cols = n
		} else if cols != n {
			return nil, errors.New("all rows must have same number of columns")
		}

		if n == 0 {
			return nil, errors.New("must have at least one column")
		}
		// name is always required
		if rec[0] == "" {
			continue NEXT
		}

		switch n {
		case 1:
			refs = append(refs, ResourceRef{
				Name: rec[0],
			})
		case 2:
			// namespace rec[1] is not required
			refs = append(refs, ResourceRef{
				Name:      rec[0],
				Namespace: rec[1],
			})
		case 3:
			// Kind is required
			if rec[2] == "" {
				continue NEXT
			}
			refs = append(refs, ResourceRef{
				Name:      rec[0],
				Namespace: rec[1],
				Kind:      rec[2],
			})
		case 4:
			// Kind is required
			if rec[2] == "" {
				continue NEXT
			}
			gv := rec[3]
			idx := strings.Index(gv, "/")
			if idx == -1 {
				idx = len(gv)
			}
			refs = append(refs, ResourceRef{
				Name:      rec[0],
				Namespace: rec[1],
				Kind:      rec[2],
				APIGroup:  gv[:idx],
			})
		default:
			return nil, fmt.Errorf("maximum 4 columns can be present, found %d", n)
		}
	}
	return refs, nil
}

func (finder ObjectFinder) Get(ref *rsapi.ObjectRef) (*unstructured.Unstructured, error) {
	gvk := schema.FromAPIVersionAndKind(ref.Target.APIVersion, ref.Target.Kind)

	objkey := client.ObjectKey{Name: ref.Name}
	opts := client.ListOptions{}
	namespaced, err := finder.isNamespaced(gvk)
	if err != nil {
		return nil, err
	}
	if namespaced {
		opts.Namespace = ref.Namespace
		objkey.Namespace = ref.Namespace
	}

	if ref.Selector != nil {
		sel, err := metav1.LabelSelectorAsSelector(ref.Selector)
		if err != nil {
			return nil, err
		}
		opts.LabelSelector = sel

		var objects unstructured.UnstructuredList
		objects.SetGroupVersionKind(gvk)
		err = finder.Client.List(context.TODO(), &objects, &opts)
		if err != nil {
			return nil, err
		}
		return getTheObject(gvk, pointer.ToUnstructuredP(objects.Items))
	}

	var object unstructured.Unstructured
	err = finder.Client.Get(context.TODO(), objkey, &object)
	if err != nil {
		return nil, err
	}
	return &object, nil
}

func (finder ObjectFinder) Locate(locator *rsapi.ObjectLocator, edgeList []rsapi.NamedEdge) (*unstructured.Unstructured, error) {
	src, err := finder.Get(&locator.Src)
	if err != nil {
		return nil, err
	}
	if len(locator.Path) == 0 {
		return src, nil
	}

	m := make(map[string]*rsapi.NamedEdge)
	for i, entry := range edgeList {
		m[entry.Name] = &edgeList[i]
	}

	from := locator.Src.Target
	edges := make([]*Edge, 0, len(locator.Path))
	for _, path := range locator.Path {
		e, ok := m[path]
		if !ok {
			return nil, fmt.Errorf("path %s not found in edge list", path)
		}

		srcGVK := schema.FromAPIVersionAndKind(e.Src.APIVersion, e.Src.Kind)
		dstGVK := schema.FromAPIVersionAndKind(e.Dst.APIVersion, e.Dst.Kind)
		if e.Src == from {
			edges = append(edges, &Edge{
				Src:        srcGVK,
				Dst:        dstGVK,
				W:          0,
				Connection: e.Connection,
				Forward:    true,
			})
			from = e.Dst
		} else if e.Dst == from {
			edges = append(edges, &Edge{
				Src:        dstGVK,
				Dst:        srcGVK,
				W:          0,
				Connection: e.Connection,
				Forward:    false,
			})
			from = e.Src
		} else {
			return nil, fmt.Errorf("edge %s has no connection with resource %v", path, from)
		}
	}

	objects, err := finder.List(src, edges)
	if err != nil {
		return nil, err
	}

	return getTheObject(edges[len(edges)-1].Dst, objects)
}

func getTheObject(gvk schema.GroupVersionKind, objects []*unstructured.Unstructured) (*unstructured.Unstructured, error) {
	switch len(objects) {
	case 0:
		return nil, kerr.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind, // actually only uses Kind not resource
		}, "")
	case 1:
		return objects[0], nil
	default:
		names := make([]string, 0, len(objects))
		for _, obj := range objects {
			name, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return nil, err
			}
			names = append(names, name)
		}
		return nil, fmt.Errorf("multiple matching %v objects found %s", gvk, strings.Join(names, ","))
	}
}
