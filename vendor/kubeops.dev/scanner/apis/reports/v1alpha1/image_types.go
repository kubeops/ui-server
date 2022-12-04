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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
)

const (
	ResourceKindImage = "Image"
	ResourceImage     = "image"
	ResourceImages    = "images"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=images,singular=image,scope=Cluster
type Image struct {
	metav1.TypeMeta `json:",inline"`
	// Request describes the attributes for the graph request.
	// +optional
	Request *ImageRequest `json:"request,omitempty"`
	// Response describes the attributes for the graph response.
	// +optional
	Response *ImageResponse `json:"response,omitempty"`
}

type ImageRequest struct {
	kmapi.ObjectInfo `json:",inline"`
}

type ImageResponse struct {
	Images []kmapi.ImageInfo `json:"images,omitempty"`
}
