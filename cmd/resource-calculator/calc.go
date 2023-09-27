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

package resource_calculator

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/metadata"
	"kmodules.xyz/client-go/cluster"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/parser"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	catalogv1alpha1 "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	"kubedb.dev/apimachinery/apis/kubedb"
	kubedbv1alpha1 "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	kubedbv1alpha2 "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	"kubedb.dev/installer/catalog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CalcProjectQuotaStatus(mgr ctrl.Manager, obj client.Object) (*v1alpha1.ProjectQuota, error) {
	kc := mgr.GetClient()
	clientSet, err := cs.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	// 1. Get the object for which the reconciler triggered
	var unstructuredResource unstructured.Unstructured
	err = kc.Get(context.Background(), client.ObjectKey{Name: obj.GetName()}, &unstructuredResource)
	if err != nil {
		return nil, err
	}

	// 2. Get the projectId for that namespace
	projectId, _, err := cluster.GetProjectId(kc, obj.GetNamespace())
	if err != nil {
		return nil, err
	}

	// 3. Get the projectQuota using the projectId
	var projectQuota v1alpha1.ProjectQuota
	err = kc.Get(context.TODO(), client.ObjectKey{Name: projectId}, &projectQuota)
	if err != nil {
		return nil, err
	}

	// 4. Calculation
	catalogMap, err := LoadCatalog(clientSet, false)
	if err != nil {
		return nil, err
	}
	topology, err := core_util.DetectTopology(context.TODO(), metadata.NewForConfigOrDie(mgr.GetConfig()))
	if err != nil {
		return nil, err
	}
	content := unstructuredResource.UnstructuredContent()
	gvk := unstructuredResource.GroupVersionKind()

	if gvk.Group == kubedb.GroupName && gvk.Version == kubedbv1alpha1.SchemeGroupVersion.Version {
		content, err = Convert_kubedb_v1alpha1_To_v1alpha2(unstructuredResource, catalogMap, topology)
		if err != nil {
			return nil, err
		}
	}

	for idx, quota := range projectQuota.Status.Quotas {
		if quota.Group == gvk.Group && quota.Kind == gvk.Kind {
			rr, err := resourcemetrics.AppResourceLimits(content)
			if err != nil {
				return nil, err
			}
			quota.Used = api.AddResourceList(rr, quota.Used)

			projectQuota.Status.Quotas[idx] = quota
			break
		}
	}

	return &projectQuota, nil
}

const TerminationPolicyPause kubedbv1alpha2.TerminationPolicy = "Pause"

type KindVersion struct {
	Kind    string
	Version string
}

func Convert_kubedb_v1alpha1_To_v1alpha2(item unstructured.Unstructured, catalogmap map[KindVersion]interface{}, topology *core_util.Topology) (map[string]interface{}, error) {
	gvk := item.GroupVersionKind()

	switch gvk.Kind {
	case kubedbv1alpha1.ResourceKindElasticsearch:
		var in kubedbv1alpha1.Elasticsearch
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.Elasticsearch
		if err := kubedbv1alpha1.Convert_v1alpha1_Elasticsearch_To_v1alpha2_Elasticsearch(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		if cv, ok := catalogmap[KindVersion{
			Kind:    gvk.Kind,
			Version: out.Spec.Version,
		}]; ok {
			out.SetDefaults(cv.(*catalogv1alpha1.ElasticsearchVersion), topology)
		} else {
			return nil, fmt.Errorf("unknown %v version %s", gvk, out.Spec.Version)
		}
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindEtcd:
		var in kubedbv1alpha1.Etcd
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.Etcd
		if err := kubedbv1alpha1.Convert_v1alpha1_Etcd_To_v1alpha2_Etcd(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults()
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindMariaDB:
		var in kubedbv1alpha1.MariaDB
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.MariaDB
		if err := kubedbv1alpha1.Convert_v1alpha1_MariaDB_To_v1alpha2_MariaDB(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults(topology)
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindMemcached:
		var in kubedbv1alpha1.Memcached
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.Memcached
		if err := kubedbv1alpha1.Convert_v1alpha1_Memcached_To_v1alpha2_Memcached(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults()
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindMongoDB:
		var in kubedbv1alpha1.MongoDB
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.MongoDB
		if err := kubedbv1alpha1.Convert_v1alpha1_MongoDB_To_v1alpha2_MongoDB(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		if cv, ok := catalogmap[KindVersion{
			Kind:    gvk.Kind,
			Version: out.Spec.Version,
		}]; ok {
			out.SetDefaults(cv.(*catalogv1alpha1.MongoDBVersion), topology)
		} else {
			return nil, fmt.Errorf("unknown %v version %s", gvk, out.Spec.Version)
		}
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindMySQL:
		var in kubedbv1alpha1.MySQL
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.MySQL
		if err := kubedbv1alpha1.Convert_v1alpha1_MySQL_To_v1alpha2_MySQL(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults(topology)
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindPerconaXtraDB:
		var in kubedbv1alpha1.PerconaXtraDB
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.PerconaXtraDB
		if err := kubedbv1alpha1.Convert_v1alpha1_PerconaXtraDB_To_v1alpha2_PerconaXtraDB(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults(topology)
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindPostgres:
		var in kubedbv1alpha1.Postgres
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.Postgres
		if err := kubedbv1alpha1.Convert_v1alpha1_Postgres_To_v1alpha2_Postgres(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		if cv, ok := catalogmap[KindVersion{
			Kind:    gvk.Kind,
			Version: out.Spec.Version,
		}]; ok {
			out.SetDefaults(cv.(*catalogv1alpha1.PostgresVersion), topology)
		} else {
			return nil, fmt.Errorf("unknown %v version %s", gvk, out.Spec.Version)
		}
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}
		if out.Spec.LeaderElection != nil && out.Spec.LeaderElection.Period.Milliseconds() == 0 {
			out.Spec.LeaderElection.Period = metav1.Duration{Duration: 300 * time.Millisecond}
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)

	case kubedbv1alpha1.ResourceKindRedis:
		var in kubedbv1alpha1.Redis
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &in); err != nil {
			return nil, err
		}
		var out kubedbv1alpha2.Redis
		if err := kubedbv1alpha1.Convert_v1alpha1_Redis_To_v1alpha2_Redis(&in, &out, nil); err != nil {
			return nil, err
		}
		out.APIVersion = kubedbv1alpha2.SchemeGroupVersion.String()
		out.Kind = in.Kind
		out.SetDefaults(topology)
		out.ObjectMeta = metav1.ObjectMeta{
			Name:            out.GetName(),
			Namespace:       out.GetNamespace(),
			Labels:          out.Labels,
			Annotations:     out.Annotations,
			OwnerReferences: out.OwnerReferences,
		}
		if out.Annotations != nil {
			delete(out.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
		if out.Spec.TerminationPolicy == TerminationPolicyPause {
			out.Spec.TerminationPolicy = kubedbv1alpha2.TerminationPolicyHalt
		}

		return runtime.DefaultUnstructuredConverter.ToUnstructured(&out)
	}
	return nil, fmt.Errorf("can't convert %v to v1alpha2", gvk)
}

func LoadCatalog(client cs.Interface, local bool) (map[KindVersion]interface{}, error) {
	catalogversions, err := parser.ListFSResources(catalog.FS())
	if err != nil {
		return nil, err
	}
	catalogmap := map[KindVersion]interface{}{}
	for _, r := range catalogversions {
		key := r.Object.GetObjectKind().GroupVersionKind()
		key.Kind = strings.TrimSuffix(key.Kind, "Version")

		switch key.Kind {
		case kubedbv1alpha1.ResourceKindElasticsearch:
			var in catalogv1alpha1.ElasticsearchVersion
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Object.UnstructuredContent(), &in); err != nil {
				return nil, err
			}
			catalogmap[KindVersion{
				Kind:    key.Kind,
				Version: r.Object.GetName(),
			}] = &in

		case kubedbv1alpha1.ResourceKindMongoDB:
			var in catalogv1alpha1.MongoDBVersion
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Object.UnstructuredContent(), &in); err != nil {
				return nil, err
			}
			catalogmap[KindVersion{
				Kind:    key.Kind,
				Version: r.Object.GetName(),
			}] = &in

		case kubedbv1alpha1.ResourceKindPostgres:
			var in catalogv1alpha1.PostgresVersion
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Object.UnstructuredContent(), &in); err != nil {
				return nil, err
			}
			catalogmap[KindVersion{
				Kind:    key.Kind,
				Version: r.Object.GetName(),
			}] = &in

		}
	}

	if !local {
		// load custom ElasticsearchVersions from cluster
		if items, err := client.CatalogV1alpha1().ElasticsearchVersions().List(context.TODO(), metav1.ListOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			for i, item := range items.Items {
				kv := KindVersion{
					Kind:    kubedbv1alpha1.ResourceKindElasticsearch,
					Version: item.GetName(),
				}
				if _, ok := catalogmap[kv]; !ok {
					catalogmap[kv] = &items.Items[i]
				}
			}
		}

		// load custom MongoDBVersions from cluster
		if items, err := client.CatalogV1alpha1().MongoDBVersions().List(context.TODO(), metav1.ListOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			for i, item := range items.Items {
				kv := KindVersion{
					Kind:    kubedbv1alpha1.ResourceKindMongoDB,
					Version: item.GetName(),
				}
				if _, ok := catalogmap[kv]; !ok {
					catalogmap[kv] = &items.Items[i]
				}
			}
		}

		// load custom PostgresVersions from cluster
		if items, err := client.CatalogV1alpha1().PostgresVersions().List(context.TODO(), metav1.ListOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			for i, item := range items.Items {
				kv := KindVersion{
					Kind:    kubedbv1alpha1.ResourceKindPostgres,
					Version: item.GetName(),
				}
				if _, ok := catalogmap[kv]; !ok {
					catalogmap[kv] = &items.Items[i]
				}
			}
		}
	}

	return catalogmap, nil
}
