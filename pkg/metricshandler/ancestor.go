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

package metricshandler

import (
	"context"

	"kubeops.dev/ui-server/pkg/graph"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
)

func (mc *Collector) collectPodAncestorMetrics() error {
	var pods core.PodList
	err := mc.kc.List(context.TODO(), &pods)
	if err != nil {
		return err
	}

	family := mc.generators[0].Generate(nil)
	for _, pod := range pods.Items {
		g, err := mc.getResourceGraph(pod.ObjectMeta)
		if err != nil {
			return err
		}
		family.Metrics = append(family.Metrics, getMetricsForSinglePod(g, pod.Name)...)
	}

	mc.store.Add(family)
	return nil
}

func (mc *Collector) getResourceGraph(podMeta metav1.ObjectMeta) (*v1alpha1.ResourceGraphResponse, error) {
	src := kmapi.ObjectID{
		Group:     "",
		Kind:      "Pod",
		Namespace: podMeta.Namespace,
		Name:      podMeta.Name,
	}

	return graph.ResourceGraph(mc.kc.RESTMapper(), src, []kmapi.EdgeLabel{
		kmapi.EdgeLabelOffshoot,
	})
}

func getMetricsForSinglePod(g *v1alpha1.ResourceGraphResponse, podName string) []*metric.Metric {
	if g == nil || g.Resources == nil || g.Connections == nil {
		return nil
	}
	podID := getPodID(g.Resources)
	if podID == -1 {
		return nil
	}

	var metrics []*metric.Metric
	mp := make(map[v1alpha1.ObjectPointer]bool)

	add := func(o v1alpha1.ObjectPointer) {
		if o.ResourceID == podID {
			return
		}
		_, exists := mp[o]
		if !exists {
			appGVK := g.Resources[o.ResourceID]
			metrics = append(metrics, &metric.Metric{
				LabelKeys: []string{
					"group",
					"kind",
					"app",
					"namespace",
					"pod",
				},
				LabelValues: []string{
					appGVK.Group,
					appGVK.Kind,
					o.Name,
					o.Namespace,
					podName,
				},
				Value: float64(1),
			})
			mp[o] = true
		}
	}

	for _, c := range g.Connections {
		add(c.Source)
		add(c.Target)
	}
	return metrics
}

func getPodID(gr []kmapi.ResourceID) int {
	podID := -1
	for i := range gr {
		res := gr[i]
		if res.Group == "" && res.Version == "v1" && res.Kind == "Pod" {
			podID = i
			break
		}
	}
	return podID
}
