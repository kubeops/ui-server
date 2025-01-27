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
	meta_util "kmodules.xyz/client-go/meta"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

func ConvertChartTemplates(tpls []releasesapi.ChartTemplate, format meta_util.DataFormat) ([]releasesapi.ChartTemplateOutput, error) {
	var out []releasesapi.ChartTemplateOutput

	for _, tpl := range tpls {
		entry := releasesapi.ChartTemplateOutput{
			ChartSourceRef: tpl.ChartSourceRef,
			ReleaseName:    tpl.ReleaseName,
			Namespace:      tpl.Namespace,
			Manifest:       tpl.Manifest,
		}

		for _, crd := range tpl.CRDs {
			data, err := meta_util.Marshal(crd, format)
			if err != nil {
				return nil, err
			}
			entry.CRDs = append(entry.CRDs, releasesapi.BucketFileOutput{
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
			entry.Resources = append(entry.Resources, releasesapi.ResourceFile{
				Filename: r.Filename,
				Key:      r.Key,
				Data:     string(data),
			})
		}
		out = append(out, entry)
	}

	return out, nil
}
