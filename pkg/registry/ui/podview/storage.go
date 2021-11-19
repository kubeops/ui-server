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

package podview

import (
	"context"

	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	"kubeops.dev/ui-server/pkg/prometheus"

	promapi "github.com/prometheus/client_golang/api"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	rsapi "kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	a         authorizer.Authorizer
	pc        promapi.Client
	convertor rest.TableConvertor
}

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Lister = &Storage{}
var _ rest.Getter = &Storage{}

func NewStorage(kc client.Client, a authorizer.Authorizer, pc promapi.Client) *Storage {
	return &Storage{
		kc: kc,
		a:  a,
		pc: pc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    uiv1alpha1.GroupName,
			Resource: uiv1alpha1.ResourcePodViews,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return uiv1alpha1.GroupVersion.WithKind(uiv1alpha1.ResourceKindPodView)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &uiv1alpha1.PodView{}
}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	var pod core.Pod
	err := r.kc.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &pod)
	if err != nil {
		return nil, err
	}

	return toPodView(&pod)
}

func toPodView(pod *core.Pod) (*uiv1alpha1.PodView, error) {
	podview := uiv1alpha1.PodView{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: pod.ObjectMeta,
		Spec: uiv1alpha1.PodViewSpec{
			Resources: uiv1alpha1.ResourceView{
				Limits:   nil,
				Requests: nil,
				Usage:    nil,
			},
			Containers: nil,
		},
		Status: pod.Status,
	}
	podview.SelfLink = ""
	podview.ManagedFields = nil
	delete(podview.ObjectMeta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")

	var limits, requests core.ResourceList

	podview.Spec.Containers = make([]uiv1alpha1.ContainerView, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		limits = rsapi.AddResourceList(limits, c.Resources.Limits)
		requests = rsapi.AddResourceList(requests, c.Resources.Requests)

		podview.Spec.Containers = append(podview.Spec.Containers, uiv1alpha1.ContainerView{
			Name:       c.Name,
			Image:      c.Image,
			Command:    c.Command,
			Args:       c.Args,
			WorkingDir: c.WorkingDir,
			Ports:      c.Ports,
			EnvFrom:    c.EnvFrom,
			Env:        c.Env,
			Resources: uiv1alpha1.ResourceView{
				Limits:   c.Resources.Limits,
				Requests: c.Resources.Requests,
				Usage:    nil,
			},
			VolumeMounts:             c.VolumeMounts,
			VolumeDevices:            c.VolumeDevices,
			LivenessProbe:            c.LivenessProbe,
			ReadinessProbe:           c.ReadinessProbe,
			StartupProbe:             c.StartupProbe,
			Lifecycle:                c.Lifecycle,
			TerminationMessagePath:   c.TerminationMessagePath,
			TerminationMessagePolicy: c.TerminationMessagePolicy,
			ImagePullPolicy:          c.ImagePullPolicy,
			SecurityContext:          c.SecurityContext,
			Stdin:                    c.Stdin,
			StdinOnce:                c.StdinOnce,
			TTY:                      c.TTY,
		})
	}
	for _, c := range pod.Spec.InitContainers {
		limits = rsapi.MaxResourceList(limits, c.Resources.Limits)
		requests = rsapi.MaxResourceList(requests, c.Resources.Requests)
	}

	usage, err := prometheus.GetPodResourceUsage(pod.ObjectMeta)
	if err != nil {
		return nil, err
	}
	podview.Spec.Resources = uiv1alpha1.ResourceView{
		Limits:   limits,
		Requests: requests,
		Usage:    usage,
	}

	return &podview, nil
}

func (r *Storage) NewList() runtime.Object {
	return &uiv1alpha1.PodViewList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	opts := client.ListOptions{Namespace: ns}
	if options != nil {
		opts.LabelSelector = options.LabelSelector
		opts.FieldSelector = options.FieldSelector
		opts.Limit = options.Limit
		opts.Continue = options.Continue
	}

	var podList core.PodList
	err := r.kc.List(ctx, &podList, &opts)
	if err != nil {
		return nil, err
	}

	podviews := make([]uiv1alpha1.PodView, 0, len(podList.Items))
	for _, pod := range podList.Items {
		podView, err := toPodView(&pod)
		if err != nil {
			return nil, err
		}
		podviews = append(podviews, *podView)
	}

	result := uiv1alpha1.PodViewList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: podList.ListMeta,
		Items:    podviews,
	}
	result.ListMeta.SelfLink = ""

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
