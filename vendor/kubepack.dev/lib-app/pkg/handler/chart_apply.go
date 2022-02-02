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

package handler

import (
	"encoding/json"
	"errors"

	"kubepack.dev/kubepack/pkg/lib"
	appapi "kubepack.dev/lib-app/api/v1alpha1"
	"kubepack.dev/lib-app/pkg/editor"
	"kubepack.dev/lib-helm/pkg/action"
	"kubepack.dev/lib-helm/pkg/storage/driver"
	"kubepack.dev/lib-helm/pkg/values"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"helm.sh/helm/v3/pkg/release"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
)

func ApplyResource(f cmdutil.Factory, model map[string]interface{}, skipCRds bool) (*release.Release, error) {
	var tm appapi.ModelMetadata
	err := meta_util.DecodeObject(model, &tm)
	if err != nil {
		return nil, errors.New("failed to parse Metadata for values")
	}

	ed, err := resourceeditors.LoadByName(resourceeditors.DefaultEditorName(tm.Resource.GroupVersionResource()))
	if err != nil {
		return nil, err
	}

	deployer, err := action.NewDeployer(f, tm.Release.Namespace, driver.ApplicationsDriverName)
	if err != nil {
		return nil, err
	}

	deployer.WithRegistry(lib.DefaultRegistry)
	var opts action.DeployOptions
	opts.ChartURL = ed.Spec.UI.Editor.URL
	opts.ChartName = ed.Spec.UI.Editor.Name
	opts.Version = ed.Spec.UI.Editor.Version

	var vals map[string]interface{}
	if _, ok := model["patch"]; ok {
		// NOTE: Makes an assumption that this is a "edit" apply
		cfg, err := f.ToRESTConfig()
		if err != nil {
			return nil, err
		}
		tpl, err := editor.LoadEditorModel(cfg, lib.DefaultRegistry, appapi.ModelMetadata{
			Metadata: tm.Metadata,
		})
		if err != nil {
			return nil, err
		}

		p3 := struct {
			Patch jsonpatch.Patch `json:"patch"`
		}{}

		data, err := json.Marshal(model)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &p3)
		if err != nil {
			return nil, err
		}

		original, err := json.Marshal(tpl.Values.Object)
		if err != nil {
			return nil, err
		}

		modified, err := p3.Patch.ApplyWithOptions(original, &jsonpatch.ApplyOptions{
			SupportNegativeIndices:   jsonpatch.SupportNegativeIndices,
			AccumulatedCopySizeLimit: jsonpatch.AccumulatedCopySizeLimit,
			AllowMissingPathOnRemove: true,
			EnsurePathExistsOnAdd:    false,
		})
		if err != nil {
			return nil, err
		}

		var mod map[string]interface{}
		err = json.Unmarshal(modified, &mod)
		if err != nil {
			return nil, err
		}
		vals = mod
	} else {
		vals = model
	}
	opts.Values = values.Options{
		ReplaceValues: vals,
	}

	opts.DryRun = false
	opts.DisableHooks = false
	opts.Replace = false
	opts.Wait = false
	opts.Timeout = 0
	opts.Description = "Apply editor"
	opts.Devel = false
	opts.Namespace = tm.Release.Namespace
	opts.ReleaseName = tm.Release.Name
	opts.Atomic = false
	opts.SkipCRDs = skipCRds
	opts.SubNotes = false
	opts.DisableOpenAPIValidation = false
	opts.IncludeCRDs = false

	deployer.WithOptions(opts)

	rls, _, err := deployer.Run()
	return rls, err
}

func DeleteResource(f cmdutil.Factory, release appapi.ObjectMeta) (*release.UninstallReleaseResponse, error) {
	cmd, err := action.NewUninstaller(f, release.Namespace, driver.ApplicationsDriverName)
	if err != nil {
		return nil, err
	}

	cmd.WithReleaseName(release.Name)
	cmd.WithOptions(action.UninstallOptions{
		DisableHooks: false,
		DryRun:       false,
		KeepHistory:  false,
		Timeout:      0,
	})
	return cmd.Run()
}
