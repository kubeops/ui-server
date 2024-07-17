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

//nolint:unused
package main

import (
	"context"
	"fmt"

	"kubeops.dev/ui-server/pkg/apiserver"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/registry/identity/selfsubjectnamespaceaccessreview"

	authorization "k8s.io/api/authorization/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourcedescriptors"
	"kmodules.xyz/resource-metadata/hub/resourceoutlines"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func NewClient() (kubernetes.Interface, client.Client, error) {
	ctrl.SetLogger(klogr.New()) // nolint:staticcheck
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	hc, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, hc)
	if err != nil {
		return nil, nil, err
	}

	kc := kubernetes.NewForConfigOrDie(cfg)
	rtc, err := client.New(cfg, client.Options{
		Scheme: apiserver.Scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
	return kc, rtc, err
}

func main() {
	kc, rtc, err := NewClient()
	if err != nil {
		panic(err)
	}
	s := selfsubjectnamespaceaccessreview.NewStorage(kc, rtc)

	ctx := context.Background()
	ctx = request.WithUser(ctx, &user.DefaultInfo{
		Name: "system:serviceaccount:kube-system:lke-admin",
		UID:  "ea0d4e91-3630-4c36-a0a4-43a27b3a6db8",
		Groups: []string{
			"system:serviceaccounts",
			"system:serviceaccounts:kube-system",
			"system:authenticated",
		},
		Extra: nil,
	})

	in := v1alpha1.SelfSubjectNamespaceAccessReview{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v1alpha1.SelfSubjectNamespaceAccessReviewSpec{
			ResourceAttributes: []authorization.ResourceAttributes{
				{
					Namespace:   "",
					Verb:        "get",
					Group:       "apps",
					Version:     "v1",
					Resource:    "deployments",
					Subresource: "",
					Name:        "",
				},
			},
			NonResourceAttributes: nil,
		},
		Status: v1alpha1.SubjectAccessNamespaceReviewStatus{},
	}

	out, err := s.Create(ctx, &in, nil, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", out)
}

func main_() {
	if err := ListResourceLayouts(); err != nil {
		panic(err)
	}
}

func ListResourceLayouts() error {
	_, kc, err := NewClient()
	if err != nil {
		return err
	}

	objs := resourceoutlines.List()

	items := make([]rsapi.ResourceLayout, 0, len(objs))
	for _, obj := range objs {
		layout, err := layouts.GetResourceLayout(kc, &obj)
		if err != nil {
			return kerr.NewInternalError(err)
		}
		items = append(items, *layout)
	}
	fmt.Println("Len:", len(items))
	return nil
}

func findConfigMapForPod() error {
	_, kc, err := NewClient()
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

func findServiceForServiceMonitor() error {
	_, kc, err := NewClient()
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
		Resource: "servicemonitors",
	})
	if err != nil {
		return err
	}

	var src unstructured.Unstructured
	src.SetAPIVersion("monitoring.coreos.com/v1")
	src.SetKind("ServiceMonitor")

	key := client.ObjectKey{
		Namespace: "default",
		Name:      "mongo-stats",
	}
	err = kc.Get(context.TODO(), key, &src)
	if err != nil {
		return err
	}

	finder := graph.ObjectFinder{Client: kc}

	for _, c := range rd.Spec.Connections {
		if c.Target.Kind == "Service" {
			result, err := finder.ListConnectedObjectIDs(&src, []rsapi.ResourceConnection{c})
			if err != nil {
				return err
			}
			fmt.Printf("%+v\n", result)
		}
	}
	return nil
}

func findServiceMonitorForPrometheus() error {
	_, kc, err := NewClient()
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
