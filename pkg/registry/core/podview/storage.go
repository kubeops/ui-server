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
	"errors"
	"strings"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
	clustermeta "kmodules.xyz/client-go/cluster"
	mu "kmodules.xyz/client-go/meta"
	promclient "kmodules.xyz/monitoring-agent-api/client"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	rmapi "kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	a         authorizer.Authorizer
	builder   *promclient.ClientBuilder
	gr        schema.GroupResource
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, a authorizer.Authorizer, builder *promclient.ClientBuilder) *Storage {
	s := &Storage{
		kc:      kc,
		a:       a,
		builder: builder,
		gr: schema.GroupResource{
			Group:    "",
			Resource: "pods",
		},
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rscoreapi.GroupName,
			Resource: rscoreapi.ResourcePodViews,
		}),
	}
	return s
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rscoreapi.SchemeGroupVersion.WithKind(rscoreapi.ResourceKindPodView)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rscoreapi.ResourceKindPodView)
}

func (r *Storage) New() runtime.Object {
	return &rscoreapi.PodView{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		Namespace:       ns,
		APIGroup:        r.gr.Group,
		Resource:        r.gr.Resource,
		Name:            name,
		ResourceRequest: true,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return nil, apierrors.NewForbidden(r.gr, name, errors.New(why))
	}

	var pod core.Pod
	err = r.kc.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &pod)
	if err != nil {
		return nil, err
	}

	return r.toPodView(&pod), nil
}

func (r *Storage) toPodView(pod *core.Pod) *rscoreapi.PodView {
	result := rscoreapi.PodView{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: *pod.ObjectMeta.DeepCopy(),
		Spec: rscoreapi.PodViewSpec{
			Resources: rscoreapi.ResourceView{
				Limits:   nil,
				Requests: nil,
				Usage:    nil,
			},
			Containers: nil,
		},
		Status: pod.Status,
	}
	result.UID = "pdvw-" + pod.GetUID()
	result.ManagedFields = nil
	result.OwnerReferences = nil
	result.Finalizers = nil
	delete(result.ObjectMeta.Annotations, mu.LastAppliedConfigAnnotation)

	var limits, requests core.ResourceList

	result.Spec.Containers = make([]rscoreapi.ContainerView, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		limits = rmapi.AddResourceList(limits, c.Resources.Limits)
		requests = rmapi.AddResourceList(requests, c.Resources.Requests)

		result.Spec.Containers = append(result.Spec.Containers, rscoreapi.ContainerView{
			Name:       c.Name,
			Image:      c.Image,
			Command:    c.Command,
			Args:       c.Args,
			WorkingDir: c.WorkingDir,
			Ports:      c.Ports,
			EnvFrom:    c.EnvFrom,
			Env:        c.Env,
			Resources: rscoreapi.ResourceView{
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
		limits = rmapi.MaxResourceList(limits, c.Resources.Limits)
		requests = rmapi.MaxResourceList(requests, c.Resources.Requests)
	}

	rv := rscoreapi.ResourceView{
		Limits:   limits,
		Requests: requests,
	}

	pc, err := r.builder.GetPrometheusClient()
	if err != nil {
		klog.ErrorS(err, "failed to create Prometheus client")
	}
	if pc != nil {
		rv.Usage = promclient.GetPodResourceUsage(pc, pod.ObjectMeta)
	}
	{
		// storage
		storageReq := resource.Quantity{Format: resource.BinarySI}
		storageCap := resource.Quantity{Format: resource.BinarySI}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				var pvc core.PersistentVolumeClaim
				if err := r.kc.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: vol.PersistentVolumeClaim.ClaimName}, &pvc); err == nil {
					storageReq.Add(*pvc.Spec.Resources.Requests.Storage())
					storageCap.Add(*pvc.Status.Capacity.Storage())
					if pc != nil {
						tmp := rv.Usage[core.ResourceStorage]
						tmp.Add(promclient.GetPVCUsage(pc, pvc.ObjectMeta))
						rv.Usage[core.ResourceStorage] = tmp
					}
				}
			}
		}
		rv.Requests[core.ResourceStorage] = storageReq
		rv.Limits[core.ResourceStorage] = storageCap
	}
	result.Spec.Resources = rv

	return &result
}

func (r *Storage) NewList() runtime.Object {
	return &rscoreapi.PodViewList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}
	// for client org user, show their own namespace only when all namespace objects is requested
	if ns == "" {
		result, err := clustermeta.IsClientOrgMember(r.kc, user)
		if err != nil {
			return nil, err
		}

		if result.IsClientOrg {
			ns = result.Namespace.Name
		}
	}

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		Namespace:       ns,
		APIGroup:        r.gr.Group,
		Resource:        r.gr.Resource,
		Name:            "",
		ResourceRequest: true,
	}

	opts := client.ListOptions{Namespace: ns}
	if options != nil {
		if options.LabelSelector != nil && !options.LabelSelector.Empty() {
			opts.LabelSelector = options.LabelSelector
		}
		if options.FieldSelector != nil && !options.FieldSelector.Empty() {
			opts.FieldSelector = options.FieldSelector
		}
		opts.Limit = options.Limit
		opts.Continue = options.Continue
	}

	var podList core.PodList
	err := r.kc.List(context.TODO(), &podList, &opts)
	if err != nil {
		return nil, err
	}

	podviews := make([]rscoreapi.PodView, 0, len(podList.Items))
	for _, pod := range podList.Items {
		attrs.Name = pod.Name
		attrs.Namespace = pod.Namespace
		decision, _, err := r.a.Authorize(context.TODO(), attrs)
		if err != nil {
			return nil, apierrors.NewInternalError(err)
		}
		if decision != authorizer.DecisionAllow {
			continue
		}

		podView := r.toPodView(&pod)
		podviews = append(podviews, *podView)
	}

	result := rscoreapi.PodViewList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: podList.ListMeta,
		Items:    podviews,
	}

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
