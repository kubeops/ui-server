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

package client

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func GetPodResourceUsage(pc promv1.API, obj metav1.ObjectMeta) core.ResourceList {
	resUsage := core.ResourceList{}

	promCPUQuery := fmt.Sprintf(`sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="%s", pod="%s", container!=""})`, obj.Namespace, obj.Name)
	promMemoryQuery := fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s", pod="%s", container!="", image!=""})`, obj.Namespace, obj.Name)
	promStorageQuery := fmt.Sprintf(`avg(container_blkio_device_usage_total{namespace="%s", pod="%s"})`, obj.Namespace, obj.Name)

	res, err := getPromQueryResult(pc, promCPUQuery)
	if err != nil {
		klog.Errorf("failed to get prometheus cpu query result, reason: %v", err)
		return resUsage
	}
	cpu := float64(0)
	for _, v := range res {
		cpu += v
	}
	cpuQuantity, err := resource.ParseQuantity(fmt.Sprintf("%.3f", cpu))
	if err != nil {
		klog.Errorf("failed to parse CPU quantity, reason: %v", err)
		return resUsage
	}
	resUsage[core.ResourceCPU] = cpuQuantity

	memory := float64(0)
	res, err = getPromQueryResult(pc, promMemoryQuery)
	if err != nil {
		klog.Errorf("failed to get prometheus memory query result, reason: %v", err)
		return resUsage
	}
	for _, v := range res {
		memory += v
	}
	memQuantity, err := resource.ParseQuantity(convertBytesToSize(memory))
	if err != nil {
		klog.Errorf("failed to parse memory quantity, reason: %v", err)
		return resUsage
	}
	resUsage[core.ResourceMemory] = memQuantity

	storage := float64(0)
	res, err = getPromQueryResult(pc, promStorageQuery)
	if err != nil {
		klog.Errorf("failed to get prometheus storage query result, reason: %v", err)
		return resUsage
	}
	for _, v := range res {
		storage += v
	}
	storageQuantity, err := resource.ParseQuantity(convertBytesToSize(storage))
	if err != nil {
		klog.Errorf("failed to parse memory quantity, reason: %v", err)
		return resUsage
	}
	resUsage[core.ResourceStorage] = storageQuantity

	return resUsage
}

func getPromQueryResult(pc promv1.API, promQuery string) (map[string]float64, error) {
	val, warn, err := pc.Query(context.Background(), promQuery, time.Now())
	if err != nil {
		return nil, err
	}
	if warn != nil {
		log.Println("Warning: ", warn)
	}

	metrics := strings.Split(val.String(), "\n")

	cpu := float64(0)

	metricsMap := make(map[string]float64)

	for _, m := range metrics {
		val := strings.Split(m, "=>")
		if len(val) != 2 {
			return nil, fmt.Errorf("metrics %q is invalid for query %s", m, promQuery)
		}
		valStr := strings.Split(val[1], "@")
		if len(valStr) != 2 {
			return nil, fmt.Errorf("metrics %q is invalid for query %s", m, promQuery)
		}
		valStr[0] = strings.Replace(valStr[0], " ", "", -1)
		metricVal, err := strconv.ParseFloat(valStr[0], 64)
		if err != nil {
			return nil, err
		}
		cpu += metricVal

		metricsMap[val[0]] = metricVal
	}

	return metricsMap, nil
}

func convertBytesToSize(b float64) string {
	ans := float64(0)
	tb := math.Pow(2, 40)
	gb := math.Pow(2, 30)
	mb := math.Pow(2, 20)
	if b >= tb {
		ans = b / tb
		return fmt.Sprintf("%vTi", math.Round(ans))
	}
	if b >= gb {
		ans = b / gb
		return fmt.Sprintf("%vGi", math.Round(ans))
	}
	ans = b / mb
	return fmt.Sprintf("%vMi", math.Round(ans))
}
