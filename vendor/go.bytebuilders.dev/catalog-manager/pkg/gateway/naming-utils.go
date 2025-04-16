/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"strconv"

	meta_util "kmodules.xyz/client-go/meta"
)

const (
	DefaultReferenceGrantName = "ace-tls-refg"
)

func GetDefaultCertRefgName() string {
	return DefaultReferenceGrantName
}

//func GetGatewayClassName() v1.ObjectName {
//	return DefaultGatewayClassName
//}
//
//func GetGatewayName(namespace string) string {
//	return meta.NameWithPrefix(ACEGatewayPrefix, namespace, 10)
//}

func GetRouteName(serviceName string) string {
	return serviceName
}

func GetRouteNameWithSuffix(serviceName, suffix string) string {
	return meta_util.NameWithSuffix(serviceName, suffix)
}

func GetListenerName(routeName string) string {
	return routeName
}

func GetListenerNameReplica(routeName string, replica int32) string {
	return routeName + "-" + strconv.Itoa(int(replica))
}

func GetBackendTLSPolicyName(serviceName string) string {
	return serviceName
}

//func GetKubeDBGatewayClassName() string {
//	return DefaultGatewayClassName
//}
