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
	"sort"
	"strings"

	"github.com/gobuffalo/flect"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kmodules.xyz/client-go/tools/parser"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

func ResourceKey(apiVersion, kind, chartName, name string) (string, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return "", err
	}

	groupPrefix := gv.Group
	groupPrefix = strings.TrimSuffix(groupPrefix, ".k8s.io")
	groupPrefix = strings.TrimSuffix(groupPrefix, ".kubernetes.io")
	// groupPrefix = strings.TrimSuffix(groupPrefix, ".x-k8s.io")
	groupPrefix = strings.Replace(groupPrefix, ".", "_", -1)
	groupPrefix = flect.Pascalize(groupPrefix)

	var nameSuffix string
	nameSuffix = strings.TrimPrefix(name, chartName)
	// we can't use - as separator since Go template does not like it
	// Go template throws an error like "unexpected bad character U+002D '-' in with"
	// ref: https://github.com/gohugoio/hugo/issues/1474
	nameSuffix = flect.Underscore(nameSuffix)
	nameSuffix = strings.Trim(nameSuffix, "_")

	result := flect.Camelize(groupPrefix + kind)
	if len(nameSuffix) > 0 {
		result += "_" + nameSuffix
	}
	return result, nil
}

func MustResourceKey(apiVersion, kind, chartName, name string) string {
	key, err := ResourceKey(apiVersion, kind, chartName, name)
	if err != nil {
		panic(err)
	}
	return key
}

func ResourceFilename(apiVersion, kind, chartName, name string) (string, string, string) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		panic(err)
	}

	groupPrefix := gv.Group
	groupPrefix = strings.TrimSuffix(groupPrefix, ".k8s.io")
	groupPrefix = strings.TrimSuffix(groupPrefix, ".kubernetes.io")
	// groupPrefix = strings.TrimSuffix(groupPrefix, ".x-k8s.io")
	groupPrefix = strings.Replace(groupPrefix, ".", "_", -1)
	groupPrefix = flect.Pascalize(groupPrefix)

	var nameSuffix string
	nameSuffix = strings.TrimPrefix(name, chartName)
	nameSuffix = strings.Replace(nameSuffix, ".", "-", -1)
	nameSuffix = strings.Trim(nameSuffix, "-")
	nameSuffix = flect.Pascalize(nameSuffix)

	return flect.Underscore(kind), flect.Underscore(kind + nameSuffix), flect.Underscore(groupPrefix + kind + nameSuffix)
}

func ListResources(chartName string, data []byte) ([]releasesapi.ResourceObject, error) {
	s1map := map[string]int{}
	s2map := map[string]int{}
	s3map := map[string]int{}

	err := parser.ProcessResources(data, func(ri parser.ResourceInfo) error {
		s1, s2, s3 := ResourceFilename(ri.Object.GetAPIVersion(), ri.Object.GetKind(), chartName, ri.Object.GetName())
		if v, ok := s1map[s1]; !ok {
			s1map[s1] = 1
		} else {
			s1map[s1] = v + 1
		}
		if v, ok := s2map[s2]; !ok {
			s2map[s2] = 1
		} else {
			s2map[s2] = v + 1
		}
		if v, ok := s3map[s3]; !ok {
			s3map[s3] = 1
		} else {
			s3map[s3] = v + 1
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var resources []releasesapi.ResourceObject

	err = parser.ProcessResources(data, func(ri parser.ResourceInfo) error {
		if ri.Object.GetNamespace() == "" {
			ri.Object.SetNamespace(core.NamespaceDefault)
		}

		s1, s2, s3 := ResourceFilename(ri.Object.GetAPIVersion(), ri.Object.GetKind(), chartName, ri.Object.GetName())
		name := s1
		if s1map[s1] > 1 {
			if s2map[s2] > 1 {
				name = s3
			} else {
				name = s2
			}
		}

		resources = append(resources, releasesapi.ResourceObject{
			Filename: name,
			Key:      MustResourceKey(ri.Object.GetAPIVersion(), ri.Object.GetKind(), chartName, ri.Object.GetName()),
			Data:     ri.Object,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Data.GetAPIVersion() == resources[j].Data.GetAPIVersion() {
			return resources[i].Data.GetKind() < resources[j].Data.GetKind()
		}
		return resources[i].Data.GetAPIVersion() < resources[j].Data.GetAPIVersion()
	})

	return resources, nil
}
