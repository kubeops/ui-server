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

	"kubeops.dev/ui-server/pkg/metricsstore"
	policystorage "kubeops.dev/ui-server/pkg/registry/policy/reports"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func collectPolicyMetrics(kc client.Client, generators []generator.FamilyGenerator, store *metricsstore.MetricsStore) error {
	if clTotal, clByType, err := collectForCluster(kc, generators[9], generators[10]); err != nil {
		return err
	} else {
		store.Add(clTotal, clByType)
	}

	if nsTotal, nsByType, err := collectForNamespace(kc, generators[11], generators[12]); err != nil {
		return err
	} else {
		store.Add(nsTotal, nsByType)
	}

	return nil
}

func collectForCluster(kc client.Client, genTotal generator.FamilyGenerator, genByType generator.FamilyGenerator) (*metric.Family, *metric.Family, error) {
	fTotal := genTotal.Generate(nil)
	fByType := genByType.Generate(nil)

	templates, err := policystorage.ListTemplates(context.TODO(), kc)
	if err != nil {
		return nil, nil, err
	}
	clusterTotal := 0
	for _, template := range templates.Items {
		constraintKind, _, err := unstructured.NestedString(template.UnstructuredContent(), "spec", "crd", "spec", "names", "kind")
		if err != nil {
			return nil, nil, err
		}
		constraints, err := policystorage.ListConstraints(context.TODO(), kc, constraintKind)
		if err != nil {
			return nil, nil, err
		}

		total := 0
		for _, constraint := range constraints.Items {
			violations, err := policystorage.GetViolationsOfConstraint(constraint)
			if err != nil {
				return nil, nil, err
			}
			total += len(violations)
		}
		mByType := metric.Metric{
			LabelKeys: []string{
				"constraint",
			},
			LabelValues: []string{
				constraintKind,
			},
			Value: float64(total),
		}
		fByType.Metrics = append(fByType.Metrics, &mByType)
		clusterTotal += total
	}
	mTotal := metric.Metric{
		Value: float64(clusterTotal),
	}
	fByType.Metrics = append(fByType.Metrics, &mTotal)
	return fTotal, fByType, nil
}

func collectForNamespace(kc client.Client, genTotal generator.FamilyGenerator, genByType generator.FamilyGenerator) (*metric.Family, *metric.Family, error) {
	fTotal := genTotal.Generate(nil)
	fByType := genByType.Generate(nil)

	templates, err := policystorage.ListTemplates(context.TODO(), kc)
	if err != nil {
		return nil, nil, err
	}
	for _, template := range templates.Items {
		cKind, _, err := unstructured.NestedString(template.UnstructuredContent(), "spec", "crd", "spec", "names", "kind")
		if err != nil {
			return nil, nil, err
		}
		constraints, err := policystorage.ListConstraints(context.TODO(), kc, cKind)
		if err != nil {
			return nil, nil, err
		}

		namespaceWiseViolation := make(map[string]int, 0)
		constraintThenNamespaceWiseViolation := make(map[string]map[string]int, 0)
		for _, constraint := range constraints.Items {
			violations, err := policystorage.GetViolationsOfConstraint(constraint)
			if err != nil {
				return nil, nil, err
			}

			for _, violation := range violations {
				if violation.Namespace != "" { // this violation occurred in a namespace-scoped object
					namespaceWiseViolation[violation.Namespace]++
					_, exist := constraintThenNamespaceWiseViolation[cKind]
					if !exist {
						constraintThenNamespaceWiseViolation[cKind] = make(map[string]int, 0)
					}
					constraintThenNamespaceWiseViolation[cKind][violation.Namespace]++
				}
			}
		}

		for ns, vCount := range namespaceWiseViolation {
			mTotal := metric.Metric{
				LabelKeys: []string{
					"namespace",
				},
				LabelValues: []string{
					ns,
				},
				Value: float64(vCount),
			}
			fTotal.Metrics = append(fTotal.Metrics, &mTotal)
		}
		for k, nsWiseViolation := range constraintThenNamespaceWiseViolation {
			for ns, vCount := range nsWiseViolation {
				mByType := metric.Metric{
					LabelKeys: []string{
						"constraint",
						"namespace",
					},
					LabelValues: []string{
						k,
						ns,
					},
					Value: float64(vCount),
				}
				fByType.Metrics = append(fByType.Metrics, &mByType)
			}
		}
	}
	return fTotal, fByType, nil
}
