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

package v1alpha1

import (
	"fmt"

	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func (dst *Workload) Duckify(srcRaw runtime.Object) error {
	switch src := srcRaw.(type) {
	case *core.ReplicationController:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "ReplicationController",
			APIVersion: core.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: src.Spec.Selector,
		}
		dst.Spec.Template = *src.Spec.Template
		return nil
	case *apps.Deployment:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		return nil
	case *apps.StatefulSet:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: apps.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		return nil
	case *apps.DaemonSet:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: apps.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		return nil
	case *batch.Job:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: batch.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		return nil
	case *batch.CronJob:
		dst.TypeMeta = metav1.TypeMeta{
			Kind:       "CronJob",
			APIVersion: batch.SchemeGroupVersion.String(),
		}
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.JobTemplate.Spec.Selector
		dst.Spec.Template = src.Spec.JobTemplate.Spec.Template
		return nil
	case *unstructured.Unstructured:
		gvk := srcRaw.GetObjectKind().GroupVersionKind()
		switch gvk {
		case apps.SchemeGroupVersion.WithKind("Deployment"),
			apps.SchemeGroupVersion.WithKind("StatefulSet"),
			apps.SchemeGroupVersion.WithKind("DaemonSet"),
			batch.SchemeGroupVersion.WithKind("Job"):
			return runtime.DefaultUnstructuredConverter.FromUnstructured(src.UnstructuredContent(), dst)
		case core.SchemeGroupVersion.WithKind("ReplicationController"):
			var obj core.ReplicationController
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(src.UnstructuredContent(), &obj); err != nil {
				return err
			}
			dst.SetGroupVersionKind(gvk)
			dst.ObjectMeta = obj.ObjectMeta
			dst.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: obj.Spec.Selector,
			}
			dst.Spec.Template = *obj.Spec.Template
			return nil
		case batch.SchemeGroupVersion.WithKind("CronJob"):
			var obj batch.CronJob
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(src.UnstructuredContent(), &obj); err != nil {
				return err
			}
			dst.SetGroupVersionKind(gvk)
			dst.ObjectMeta = obj.ObjectMeta
			dst.Spec.Selector = obj.Spec.JobTemplate.Spec.Selector
			dst.Spec.Template = obj.Spec.JobTemplate.Spec.Template
			return nil
		}
	}
	return fmt.Errorf("unknown src type %T", srcRaw)
}
