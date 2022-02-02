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

package editor

import (
	appapi "kubepack.dev/lib-app/api/v1alpha1"

	meta_util "kmodules.xyz/client-go/meta"
)

func ConvertChartTemplates(tpls []appapi.ChartTemplate, format meta_util.DataFormat) ([]appapi.ChartTemplateOutput, error) {
	var out []appapi.ChartTemplateOutput

	for _, tpl := range tpls {
		entry := appapi.ChartTemplateOutput{
			ChartRef:    tpl.ChartRef,
			Version:     tpl.Version,
			ReleaseName: tpl.ReleaseName,
			Namespace:   tpl.Namespace,
			Manifest:    tpl.Manifest,
		}

		for _, crd := range tpl.CRDs {
			data, err := meta_util.Marshal(crd, format)
			if err != nil {
				return nil, err
			}
			entry.CRDs = append(entry.CRDs, appapi.BucketFileOutput{
				URL:      crd.URL,
				Key:      crd.Key,
				Filename: crd.Filename,
				Data:     string(data),
			})
		}
		for _, r := range tpl.Resources {
			data, err := meta_util.Marshal(r.Data, format)
			if err != nil {
				return nil, err
			}
			entry.Resources = append(entry.Resources, appapi.ResourceFile{
				Filename: r.Filename,
				Data:     string(data),
			})
		}
		out = append(out, entry)
	}

	return out, nil
}
