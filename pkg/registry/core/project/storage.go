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

package project

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	"kmodules.xyz/resource-metadata/apis/shared"
	"sigs.k8s.io/controller-runtime/pkg/client"
	chartsapi "x-helm.dev/apimachinery/apis/charts/v1alpha1"
)

type Storage struct {
	kc        client.Client
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}

	gr = schema.GroupResource{
		Group:    rscoreapi.SchemeGroupVersion.Group,
		Resource: rscoreapi.ResourceProjects,
	}
)

func NewStorage(kc client.Client) *Storage {
	s := &Storage{
		kc:        kc,
		convertor: NewDefaultTableConvertor(gr),
	}
	return s
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rscoreapi.SchemeGroupVersion.WithKind(rscoreapi.ResourceKindProject)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rscoreapi.ResourceKindProject)
}

func (r *Storage) New() runtime.Object {
	return &rscoreapi.Project{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		return GetRancherProject(r.kc, name)
	}
	return nil, apierrors.NewNotFound(gr, name)
}

func (r *Storage) NewList() runtime.Object {
	return &rscoreapi.ProjectList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	var projects []rscoreapi.Project
	var err error
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		projects, err = ListRancherProjects(r.kc)
		if err != nil {
			return nil, err
		}
	}

	result := rscoreapi.ProjectList{
		TypeMeta: metav1.TypeMeta{},
		// ListMeta: nil,
		Items: projects,
	}

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func ListRancherProjects(kc client.Client) ([]rscoreapi.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list)
	if meta.IsNoMatchError(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	projects := map[string]rscoreapi.Project{}
	now := time.Now()
	for _, ns := range list.Items {
		projectId, exists := ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]
		if !exists {
			continue
		}

		project, exists := projects[projectId]
		if !exists {
			project = rscoreapi.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:              projectId,
					CreationTimestamp: metav1.NewTime(now),
					UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
					Labels: map[string]string{
						clustermeta.LabelKeyRancherFieldProjectId: projectId,
					},
				},
				Spec: rscoreapi.ProjectSpec{
					Type:       rscoreapi.ProjectUser,
					Namespaces: nil,
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clustermeta.LabelKeyRancherFieldProjectId: projectId,
						},
					},
				},
			}
		}

		if ns.CreationTimestamp.Before(&project.CreationTimestamp) {
			project.CreationTimestamp = ns.CreationTimestamp
		}

		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = rscoreapi.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = rscoreapi.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)

		projects[projectId] = project
	}

	for projectId, prj := range projects {
		var hasUseNs bool
		presets := prj.Spec.Presets
		for _, ns := range prj.Spec.Namespaces {
			if !strings.HasPrefix(ns, "cattle-project-p-") {
				hasUseNs = true
			}

			if prj.Spec.Type == rscoreapi.ProjectSystem {
				if ns == metav1.NamespaceSystem {
					var ccps chartsapi.ClusterChartPresetList
					err := kc.List(context.TODO(), &ccps)
					if err != nil && !meta.IsNoMatchError(err) {
						return nil, err
					}
					for _, x := range ccps.Items {
						presets = append(presets, shared.SourceLocator{
							Resource: kmapi.ResourceID{
								Group:   chartsapi.GroupVersion.Group,
								Version: chartsapi.GroupVersion.Version,
								Kind:    chartsapi.ResourceKindClusterChartPreset,
							},
							Ref: kmapi.ObjectReference{
								Name: x.Name,
							},
						})
					}
				}
			} else {
				var cps chartsapi.ChartPresetList
				err := kc.List(context.TODO(), &cps, client.InNamespace(ns))
				if err != nil && !meta.IsNoMatchError(err) {
					return nil, err
				}
				for _, x := range cps.Items {
					presets = append(presets, shared.SourceLocator{
						Resource: kmapi.ResourceID{
							Group:   chartsapi.GroupVersion.Group,
							Version: chartsapi.GroupVersion.Version,
							Kind:    chartsapi.ResourceKindChartPreset,
						},
						Ref: kmapi.ObjectReference{
							Name:      x.Name,
							Namespace: x.Namespace,
						},
					})
				}
			}
		}

		// drop projects where all namespaces start with cattle-project-p
		if !hasUseNs {
			delete(projects, projectId)
			continue
		}

		sort.Slice(presets, func(i, j int) bool {
			if presets[i].Ref.Namespace != presets[j].Ref.Namespace {
				return presets[i].Ref.Namespace < presets[j].Ref.Namespace
			}
			return presets[i].Ref.Name < presets[j].Ref.Name
		})

		prj.Spec.Presets = presets
		projects[projectId] = prj
	}

	if clustermeta.IsRancherManaged(kc.RESTMapper()) {
		sysProjectId, _, err := clustermeta.GetSystemProjectId(kc)
		if err != nil {
			return nil, err
		}

		var promList monitoringv1.PrometheusList
		err = kc.List(context.TODO(), &promList)
		if err != nil && !meta.IsNoMatchError(err) {
			return nil, err
		}
		for _, prom := range promList.Items {
			var projectId string
			if prom.Namespace == clustermeta.NamespaceRancherMonitoring {
				projectId = sysProjectId
			} else {
				if prom.Spec.ServiceMonitorNamespaceSelector != nil {
					projectId = prom.Spec.ServiceMonitorNamespaceSelector.MatchLabels[clustermeta.LabelKeyRancherHelmProjectId]
				}
			}

			prj, found := projects[projectId]
			if !found {
				continue
			}

			if prj.Spec.Monitoring == nil {
				prj.Spec.Monitoring = &rscoreapi.ProjectMonitoring{}
			}
			prj.Spec.Monitoring.PrometheusRef = &kmapi.ObjectReference{
				Namespace: prom.Namespace,
				Name:      prom.Name,
			}

			alertmanager, err := FindSiblingAlertManagerForPrometheus(kc, client.ObjectKeyFromObject(prom))
			if err != nil {
				return nil, err
			}
			prj.Spec.Monitoring.AlertmanagerRef = &kmapi.ObjectReference{
				Namespace: alertmanager.Namespace,
				Name:      alertmanager.Name,
			}

			if projectId == sysProjectId {
				prj.Spec.Monitoring.AlertmanagerURL = alertmanager.Spec.ExternalURL
				prj.Spec.Monitoring.PrometheusURL = prom.Spec.ExternalURL
				prj.Spec.Monitoring.GrafanaURL = strings.Replace(
					prj.Spec.Monitoring.PrometheusURL,
					"/services/http:rancher-monitoring-prometheus:9090/proxy",
					"/services/http:rancher-monitoring-grafana:80/proxy/?orgId=1",
					1)
			} else {
				prj.Spec.Monitoring.AlertmanagerURL,
					prj.Spec.Monitoring.GrafanaURL,
					prj.Spec.Monitoring.PrometheusURL = DetectProjectMonitoringURLs(kc, prom.Namespace)
			}

			projects[projectId] = prj
		}
	}

	result := make([]rscoreapi.Project, 0, len(projects))
	for _, p := range projects {
		result = append(result, p)
	}
	return result, nil
}

func FindSiblingAlertManagerForPrometheus(kc client.Client, key types.NamespacedName) (*monitoringv1.Alertmanager, error) {
	var list monitoringv1.AlertmanagerList
	err := kc.List(context.TODO(), &list, client.InNamespace(key.Namespace))
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		klog.Warningf("multiple alert manager found in namespace %s", key.Namespace)
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

func DetectProjectMonitoringURLs(kc client.Client, promNS string) (alertmanagerURL, grafanaURL, prometheusURL string) {
	var prjHelm unstructured.Unstructured
	prjHelm.SetAPIVersion("helm.cattle.io/v1alpha1")
	prjHelm.SetKind("ProjectHelmChart")
	key := client.ObjectKey{
		Name:      "project-monitoring",
		Namespace: strings.TrimSuffix(promNS, "-monitoring"),
	}
	err := kc.Get(context.TODO(), key, &prjHelm)
	if err != nil {
		return
	}

	alertmanagerURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "alertmanagerURL")
	grafanaURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "grafanaURL")
	prometheusURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "prometheusURL")
	return
}

func GetRancherProject(kc client.Client, projectId string) (*rscoreapi.Project, error) {
	projects, err := ListRancherProjects(kc)
	if err != nil {
		return nil, err
	}
	for _, prj := range projects {
		if prj.Name == projectId {
			return &prj, nil
		}
	}
	return nil, apierrors.NewNotFound(gr, projectId)
}
