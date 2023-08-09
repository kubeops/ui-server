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

package main

import (
	"context"
	"fmt"

	"kubeops.dev/ui-server/pkg/graph"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourcedescriptors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func NewClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
}

func main() {
	if err := finPrometheusForServiceMonitor(); err != nil {
		panic(err)
	}
}

func finConfigMapForPod() error {
	fmt.Println("Using kubebuilder client")
	kc, err := NewClient()
	if err != nil {
		return err
	}

	rd, err := resourcedescriptors.LoadByGVR(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	})
	if err != nil {
		return err
	}

	var src unstructured.Unstructured
	src.SetAPIVersion("v1")
	src.SetKind("Pod")

	key := client.ObjectKey{
		Namespace: "kube-system",
		Name:      "calico-node-ctlqh",
	}
	err = kc.Get(context.TODO(), key, &src)
	if err != nil {
		return err
	}

	finder := graph.ObjectFinder{Client: kc}

	for _, c := range rd.Spec.Connections {
		if c.Target.Kind == "ConfigMap" {
			result, err := finder.ListConnectedObjectIDs(&src, []rsapi.ResourceConnection{c})
			if err != nil {
				return err
			}
			fmt.Printf("%+v\n", result)
		}
	}
	return nil
}

func finPrometheusForServiceMonitor() error {
	fmt.Println("Using kubebuilder client")
	kc, err := NewClient()
	if err != nil {
		return err
	}

	/*
		apiVersion: monitoring.coreos.com/v1
		kind: Prometheus
	*/
	rd, err := resourcedescriptors.LoadByGVR(schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "prometheuses",
	})
	if err != nil {
		return err
	}

	var src unstructured.Unstructured
	src.SetAPIVersion("monitoring.coreos.com/v1")
	src.SetKind("Prometheus")

	key := client.ObjectKey{
		Namespace: "monitoring",
		Name:      "kube-prometheus-stack-prometheus",
	}
	err = kc.Get(context.TODO(), key, &src)
	if err != nil {
		return err
	}

	finder := graph.ObjectFinder{Client: kc}

	for _, c := range rd.Spec.Connections {
		if c.Target.Kind == "ServiceMonitor" {
			result, err := finder.ListConnectedObjectIDs(&src, []rsapi.ResourceConnection{c})
			if err != nil {
				return err
			}
			fmt.Printf("%+v\n", result)
		}
	}
	return nil
}
