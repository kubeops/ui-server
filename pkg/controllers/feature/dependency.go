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

package feature

import (
	"context"

	"gomodules.xyz/pointer"
	"gomodules.xyz/sets"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	cu "kmodules.xyz/client-go/client"
	"kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *frReconciler) updateAllFeatureSetDependencies(ctx context.Context) error {
	enabled := pointer.Bool(r.feature.Status.Managed) && pointer.Bool(r.feature.Status.Enabled)

	for _, name := range r.feature.Spec.Requirements.Features {
		f := &v1alpha1.Feature{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: name}, f); err != nil {
			if apierrors.IsNotFound(err) {
				// required feature isn't available yet
				continue
			}
			return err
		}

		if !pointer.Bool(f.Status.Enabled) || f.Spec.FeatureSet == r.feature.Spec.FeatureSet {
			// required feature isn't enabled or belongs to same feature set
			continue
		}

		if err := r.updateFeatureSetDependencyStatus(ctx, f.Spec.FeatureSet, enabled); err != nil {
			return err
		}

	}
	return nil
}

func (r *frReconciler) updateFeatureSetDependencyStatus(ctx context.Context, fsName string, enabled bool) error {
	fs := &v1alpha1.FeatureSet{}
	err := r.client.Get(ctx, types.NamespacedName{Name: fsName}, fs)
	if err != nil {
		return err
	}

	if enabled {
		fs.Status.Dependents = addFeatureDependents(fs.Status.Dependents, r.feature)
	} else {
		fs.Status.Dependents = removeFeatureDependents(fs.Status.Dependents, r.feature)
	}

	_, err = cu.PatchStatus(ctx, r.client, fs, func(obj client.Object) client.Object {
		in := obj.(*v1alpha1.FeatureSet)
		in.Status.Dependents = fs.Status.Dependents
		return in
	})
	return err
}

func addFeatureDependents(dependents v1alpha1.Dependents, f *v1alpha1.Feature) v1alpha1.Dependents {
	for idx := range dependents.FeatureSets {
		fs := dependents.FeatureSets[idx]
		if fs.Name == f.Spec.FeatureSet {
			dependents.FeatureSets[idx].Features = addIfNotExists(fs.Features, f.Name)
			return dependents
		}
	}

	dependents.FeatureSets = append(dependents.FeatureSets, v1alpha1.DependentFeatureSet{
		Name:     f.Spec.FeatureSet,
		Features: []string{f.Name},
	})
	return dependents
}

func removeFeatureDependents(dependents v1alpha1.Dependents, f *v1alpha1.Feature) v1alpha1.Dependents {
	dfs := make([]v1alpha1.DependentFeatureSet, 0, len(dependents.FeatureSets))
	for idx := range dependents.FeatureSets {
		fs := dependents.FeatureSets[idx]
		if fs.Name == f.Spec.FeatureSet {
			fs.Features = removeIfExists(fs.Features, f.Name)
			if len(fs.Features) > 0 {
				dfs = append(dfs, fs)
			}
		} else {
			dfs = append(dfs, fs)
		}
	}
	dependents.FeatureSets = dfs
	return dependents
}

func addIfNotExists(slice []string, s string) []string {
	return sets.NewString(slice...).Insert(s).List()
}

func removeIfExists(slice []string, s string) []string {
	return sets.NewString(slice...).Delete(s).List()
}
