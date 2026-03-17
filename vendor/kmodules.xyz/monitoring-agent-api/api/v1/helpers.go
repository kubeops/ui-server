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

package v1

import (
	"errors"
	"fmt"
	"strconv"

	"kmodules.xyz/client-go/policy/secomp"
	appcatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"

	"gomodules.xyz/pointer"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func (agent *AgentSpec) SetDefaults() {
	if agent == nil {
		return
	}

	if agent.Agent.Vendor() == VendorPrometheus {
		if agent.Prometheus == nil {
			agent.Prometheus = &PrometheusSpec{}
		}
		if agent.Prometheus.Exporter.Port == 0 {
			agent.Prometheus.Exporter.Port = PrometheusExporterPortNumber
		}
		agent.SetSecurityContextDefaults()
	}
}

func (agent *AgentSpec) SetSecurityContextDefaults() {
	sc := agent.Prometheus.Exporter.SecurityContext
	if sc == nil {
		sc = &core.SecurityContext{}
	}
	if sc.AllowPrivilegeEscalation == nil {
		sc.AllowPrivilegeEscalation = pointer.BoolP(false)
	}
	if sc.Capabilities == nil {
		sc.Capabilities = &core.Capabilities{
			Drop: []core.Capability{"ALL"},
		}
	}
	if sc.RunAsNonRoot == nil {
		sc.RunAsNonRoot = pointer.BoolP(true)
	}
	if sc.SeccompProfile == nil {
		sc.SeccompProfile = secomp.DefaultSeccompProfile()
	}
	agent.Prometheus.Exporter.SecurityContext = sc
}

func IsKnownAgentType(at AgentType) bool {
	switch at {
	case AgentPrometheus,
		AgentPrometheusOperator,
		AgentPrometheusBuiltin:
		return true
	}
	return false
}

func TricksterBackend(isDefault bool, ownerID int64, clusterUID, projectId string) string {
	if isDefault || projectId == "" {
		return fmt.Sprintf("%d.%s", ownerID, clusterUID)
	}
	return fmt.Sprintf("%d.%s.%s", ownerID, clusterUID, projectId)
}

func GrafanaDatasource(isDefault bool, clusterName, projectId string) string {
	if isDefault || projectId == "" {
		return clusterName
	}
	return fmt.Sprintf("%s-%s", clusterName, projectId)
}

func (c *ConnectionSpec) ToAppBinding() (*appcatalog.AppBinding, error) {
	var ns string
	if c.AuthSecret != nil {
		if c.AuthSecret.Namespace == "" {
			return nil, errors.New("auth secret namespace not set")
		}
		ns = c.AuthSecret.Namespace
	}
	if c.TLSSecret != nil {
		if c.TLSSecret.Namespace == "" {
			return nil, errors.New("tls secret namespace not set")
		}
		if ns != "" && ns != c.TLSSecret.Namespace {
			return nil, errors.New("tls secret namespace does not match auth secret namespace")
		}
	}

	app := appcatalog.AppBinding{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "<generated>",
			Namespace: ns,
		},
		Spec: appcatalog.AppBindingSpec{
			ClientConfig: appcatalog.ClientConfig{
				URL:                   ptr.To(c.URL),
				InsecureSkipTLSVerify: c.InsecureSkipTLSVerify,
				CABundle:              c.CABundle,
				ServerName:            c.ServerName,
			},
		},
	}
	if c.AuthSecret != nil {
		app.Spec.Secret = &appcatalog.TypedLocalObjectReference{
			Kind: "Secret", // It will create circular dependency, If we use Kubedb Constant .
			Name: c.AuthSecret.Name,
		}
	}
	if c.TLSSecret != nil {
		app.Spec.TLSSecret = &appcatalog.TypedLocalObjectReference{
			Kind: "Secret",
			Name: c.TLSSecret.Name,
		}
	}
	return &app, nil
}

func (svc ServiceSpec) ObjectKey() types.NamespacedName {
	return types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}
}

func (svc ServiceSpec) ToServiceReference() (*appcatalog.ServiceReference, error) {
	ref := appcatalog.ServiceReference{
		Scheme:    svc.Scheme,
		Namespace: svc.Namespace,
		Name:      svc.Name,
		Port:      -1,
		Path:      svc.Path,
		Query:     svc.Query,
	}
	if port, ok := IsValidPort(svc.Port); ok {
		ref.Port = int32(port)
	} else {
		return nil, fmt.Errorf("invalid service port: %q", svc.Port)
	}
	return &ref, nil
}

// IsValidPort checks if a string is a valid port number (0-65535).
func IsValidPort(portStr string) (int, bool) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return -1, false
	}
	if port >= 0 && port <= 65535 {
		return port, true
	}
	return -1, false
}
