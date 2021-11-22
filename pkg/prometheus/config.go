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

package prometheus

import (
	"flag"
	"net/url"

	promapi "github.com/prometheus/client_golang/api"
	prom_config "github.com/prometheus/common/config"
	"github.com/spf13/pflag"
	"go.bytebuilders.dev/license-verifier/info"
)

type Config struct {
	// The address where metrics will be sent
	Addr string
	// The HTTP basic authentication credentials for the targets.
	BasicAuth BasicAuth `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	// The bearer token for the targets. Deprecated in favour of
	// Authorization.Credentials.
	BearerToken string `yaml:"bearer_token,omitempty" json:"bearer_token,omitempty"`
	// The bearer token file for the targets. Deprecated in favour of
	// Authorization.CredentialsFile.
	BearerTokenFile string `yaml:"bearer_token_file,omitempty" json:"bearer_token_file,omitempty"`
	// HTTP proxy server to use to connect to the targets.
	ProxyURL string `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	// TLSConfig to use to connect to the targets.
	TLSConfig prom_config.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

// BasicAuth contains basic HTTP authentication credentials.
type BasicAuth struct {
	Username     string `yaml:"username" json:"username"`
	Password     string `yaml:"password,omitempty" json:"password,omitempty"`
	PasswordFile string `yaml:"password_file,omitempty" json:"password_file,omitempty"`
}

func NewPrometheusConfig() *Config {
	return &Config{}
}

func (p *Config) AddGoFlags(fs *flag.FlagSet) {
	fs.StringVar(&p.Addr, "prometheus.address", p.Addr, "The address of metrics storage where metrics data will be sent")

	fs.StringVar(&p.BasicAuth.Username, "prometheus.basic-auth-username", p.BasicAuth.Username, "The HTTP basic authentication username for the targets.")
	fs.StringVar(&p.BasicAuth.Password, "prometheus.basic-auth-password", p.BasicAuth.Password, "The HTTP basic authentication password for the targets.")
	fs.StringVar(&p.BasicAuth.PasswordFile, "prometheus.basic-auth-password-file", p.BasicAuth.PasswordFile, "The HTTP basic authentication password file for the targets.")

	fs.StringVar(&p.BearerToken, "prometheus.bearer-token", p.BearerToken, "The bearer token for the targets.")
	fs.StringVar(&p.BearerTokenFile, "prometheus.bearer-token-file", p.BearerTokenFile, "The bearer token file for the targets.")

	fs.StringVar(&p.ProxyURL, "prometheus.proxy-url", p.ProxyURL, "HTTP proxy server to use to connect to the targets.")

	fs.StringVar(&p.TLSConfig.CAFile, "prometheus.ca-cert-file", p.TLSConfig.CAFile, "The path of the CA cert to use for the remote metric storage.")
	fs.StringVar(&p.TLSConfig.CertFile, "prometheus.client-cert-file", p.TLSConfig.CertFile, "The path of the client cert to use for communicating with the remote metric storage.")
	fs.StringVar(&p.TLSConfig.KeyFile, "prometheus.client-key-file", p.TLSConfig.KeyFile, "The path of the client key to use for communicating with the remote metric storage.")
	fs.StringVar(&p.TLSConfig.ServerName, "prometheus.server-name", p.TLSConfig.ServerName, "The server name which will be used to verify metrics storage.")
	fs.BoolVar(&p.TLSConfig.InsecureSkipVerify, "prometheus.insecure-skip-verify", p.TLSConfig.InsecureSkipVerify, "To skip tls verification when communicating with the remote metric storage.")
}

func (p *Config) AddFlags(fs *pflag.FlagSet) {
	pfs := flag.NewFlagSet("postgres-server", flag.ExitOnError)
	p.AddGoFlags(pfs)
	fs.AddGoFlagSet(pfs)
}

func (p *Config) Validate() error {
	if p.Addr == "" {
		return nil // if prometheus.address is not set, skip validation check
	}
	httpConf, err := p.ToHTTPClientConfig()
	if err != nil {
		return err
	}
	return httpConf.Validate()
}

func (p *Config) ToHTTPClientConfig() (*prom_config.HTTPClientConfig, error) {
	cfg := prom_config.HTTPClientConfig{
		TLSConfig: prom_config.TLSConfig{
			CAFile:             p.TLSConfig.CAFile,
			CertFile:           p.TLSConfig.CertFile,
			KeyFile:            p.TLSConfig.KeyFile,
			ServerName:         p.TLSConfig.ServerName,
			InsecureSkipVerify: p.TLSConfig.InsecureSkipVerify,
		},
	}

	if p.BasicAuth.Username != "" || p.BasicAuth.Password != "" || p.BasicAuth.PasswordFile != "" {
		cfg.BasicAuth = &prom_config.BasicAuth{
			Username:     p.BasicAuth.Username,
			Password:     prom_config.Secret(p.BasicAuth.Password),
			PasswordFile: p.BasicAuth.PasswordFile,
		}
	}

	cfg.BearerToken = prom_config.Secret(p.BearerToken)
	cfg.BearerTokenFile = p.BearerTokenFile

	if p.ProxyURL != "" {
		u, err := url.Parse(p.ProxyURL)
		if err != nil {
			return nil, err
		}
		cfg.ProxyURL = prom_config.URL{URL: u}
	}

	return &cfg, nil
}

func (p *Config) NewPrometheusClient() (promapi.Client, error) {
	if p.Addr == "" {
		return nil, nil
	}

	httpConf, err := p.ToHTTPClientConfig()
	if err != nil {
		return nil, err
	}
	rt, err := prom_config.NewRoundTripperFromConfig(*httpConf, info.ProductName)
	if err != nil {
		return nil, err
	}
	return promapi.NewClient(promapi.Config{
		Address:      p.Addr,
		RoundTripper: rt,
	})
}
