/*
Copyright The Kubeshield Authors.

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

package identity

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:skipVerbs=update,delete,deleteCollection,get,list,watch,patch
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WhoAmI struct {
	metav1.TypeMeta
	// Response describes the attributes for the identity response.
	Response *WhoAmIResponse
}

// WhoAmIResponse describes an admission response.
type WhoAmIResponse struct {
	// Result contains extra details into why an admission request was denied.
	// This field IS NOT consulted in any way if "Allowed" is "true".
	User *UserInfo
}

// UserInfo holds the information about the user needed to implement the
// user.Info interface.
type UserInfo struct {
	// The name that uniquely identifies this user among all active users.
	Username string
	// A unique value that identifies this user across time. If this user is
	// deleted and another user by the same name is added, they will have
	// different UIDs.
	UID string
	// The names of groups this user is a part of.
	Groups []string
	// Any additional information provided by the authenticator.
	Extra map[string]ExtraValue
}

// ExtraValue masks the value so protobuf can generate
// +protobuf.nullable=true
// +protobuf.options.(gogoproto.goproto_stringer)=false
type ExtraValue []string

func (t ExtraValue) String() string {
	return fmt.Sprintf("%v", []string(t))
}
