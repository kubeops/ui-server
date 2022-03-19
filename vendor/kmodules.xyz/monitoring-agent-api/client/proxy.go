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

package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	appcatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"

	prom_config "github.com/prometheus/common/config"
	"k8s.io/client-go/rest"
)

func ToPrometheusConfig(cfg *rest.Config, ref appcatalog.ServiceReference) (*Config, error) {
	if err := rest.LoadTLSFiles(cfg); err != nil {
		return nil, err
	}

	certDir, err := os.MkdirTemp(os.TempDir(), "prometheus-*")
	if err != nil {
		return nil, err
	}

	var caFile, certFile, keyFile string
	if len(cfg.TLSClientConfig.CAData) > 0 {
		caFile = filepath.Join(certDir, "ca.crt")
		if err = ioutil.WriteFile(caFile, cfg.TLSClientConfig.CAData, 0o644); err != nil {
			return nil, err
		}
	}

	if len(cfg.TLSClientConfig.CertData) > 0 {
		certFile = filepath.Join(certDir, "tls.crt")
		if err = ioutil.WriteFile(certFile, cfg.TLSClientConfig.CertData, 0o644); err != nil {
			return nil, err
		}
	}

	if len(cfg.TLSClientConfig.KeyData) > 0 {
		keyFile = filepath.Join(certDir, "tls.key")
		if err = ioutil.WriteFile(keyFile, cfg.TLSClientConfig.KeyData, 0o644); err != nil {
			return nil, err
		}
	}

	return &Config{
		Addr: fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s:%s:%d/proxy/", cfg.Host, ref.Namespace, ref.Scheme, ref.Name, ref.Port),
		BasicAuth: BasicAuth{
			Username:     cfg.Username,
			Password:     cfg.Password,
			PasswordFile: "",
		},
		BearerToken:     cfg.BearerToken,
		BearerTokenFile: cfg.BearerTokenFile,
		ProxyURL:        "",
		TLSConfig: prom_config.TLSConfig{
			CAFile:             caFile,
			CertFile:           certFile,
			KeyFile:            keyFile,
			ServerName:         cfg.TLSClientConfig.ServerName,
			InsecureSkipVerify: cfg.TLSClientConfig.Insecure,
		},
	}, nil
}
