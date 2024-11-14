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

import egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"

func (i InfraTLS) MountCACerts() bool {
	return i.Issuer == TLSIssuerTypeCA ||
		i.Issuer == TLSIssuerTypeLEStaging ||
		(i.Issuer == TLSIssuerTypeExternal && i.CA != nil && i.CA.Cert != "")
}

func (gwp GatewayParameter) UsesNodePort() bool {
	return gwp.ServiceType == egv1a1.ServiceTypeNodePort ||
		gwp.ServiceType == egv1a1.ServiceTypeLoadBalancer
}
