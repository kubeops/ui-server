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

package identity

import (
	"crypto/tls"
	"crypto/x509"
	"go.bytebuilders.dev/license-verifier/info"
	"io"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	identityapi "kubeops.dev/ui-server/apis/identity/v1alpha1"
	"net/http"
	"path"
)

type Client struct {
	baseURL string
	token   string
	caCert  []byte
	client  *http.Client
}

var Identity *identityapi.ClusterIdentity

func NewClient(baseURL, token string, caCert []byte) (*Client, error) {
	c := &Client{
		baseURL: baseURL,
		token:   token,
		caCert:  caCert,
	}
	if len(caCert) == 0 {
		c.client = http.DefaultClient
	} else {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			RootCAs: caCertPool,
		}
		transport := &http.Transport{TLSClientConfig: tlsConfig}
		c.client = &http.Client{Transport: transport}
	}
	return c, nil
}

func (c *Client) Identify(clusterUID string) (*identityapi.ClusterIdentityStatus, error) {

	u, err := info.APIServerAddress(c.baseURL)
	if err != nil {
		return nil, err //TODO
	}
	u.Path = path.Join(u.Path, "api/v1/clustersv2/identity", clusterUID)

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err //TODO
	}
	req.Header.Set("Content-Type", "application/json")
	// add authorization header to the req
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err //TODO
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err //TODO
	}

	if resp.StatusCode != http.StatusOK {
		return nil, apierrors.NewGenericServerResponse(
			resp.StatusCode,
			http.MethodGet,
			schema.GroupResource{Group: identityapi.GroupName, Resource: identityapi.ResourceClusterIdentities},
			"",
			string(body),
			0,
			false,
		)
	}
	var ds identityapi.ClusterIdentityStatus
	err = json.Unmarshal(body, &ds)
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

func (c *Client) GetToken() string {
	u, err := info.APIServerAddress(c.baseURL)
	if err != nil {
		return "" //TODO
	}
	clusterID := Identity.UID
	clusterName := Identity.Name
	u.Path = path.Join(u.Path, "api/v1/agent", clusterName, string(clusterID), "token")

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "" //TODO
	}
	req.Header.Set("Content-Type", "application/json")
	// add authorization header to the req
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "" //TODO
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "" //TODO
	}
	return string(body)
}
