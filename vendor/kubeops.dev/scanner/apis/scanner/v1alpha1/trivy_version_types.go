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
	"kubeops.dev/scanner/apis/shared"
)

type TrivyVersion struct {
	Version         string                `json:"Version"`
	VulnerabilityDB VulnerabilityDBStruct `json:"VulnerabilityDB"`
}

type VulnerabilityDBStruct struct {
	Version      int32       `json:"Version"`
	UpdatedAt    shared.Time `json:"UpdatedAt"`
	DownloadedAt shared.Time `json:"DownloadedAt"`
	NextUpdate   shared.Time `json:"NextUpdate"`
}
