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

package offlinelicense

import (
	"context"
	"strings"

	licenseapi "kubeops.dev/ui-server/apis/offline/v1alpha1"
	"kubeops.dev/ui-server/pkg/registry/offline/addofflinelicense"

	"github.com/google/uuid"
	verifier "go.bytebuilders.dev/license-verifier"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    licenseapi.GroupName,
			Resource: licenseapi.ResourceOfflineLicenses,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return licenseapi.SchemeGroupVersion.WithKind(licenseapi.ResourceKindOfflineLicense)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(licenseapi.ResourceKindOfflineLicense)
}

func (r *Storage) New() runtime.Object {
	return &licenseapi.OfflineLicense{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	licenseSecret, err := getLicenseSecret(ctx, r.kc, ns)
	if err != nil {
		return &licenseapi.OfflineLicense{}, err
	}

	for product, lic := range licenseSecret.Data {
		if product == name {
			certs, err := cert.ParseCertsPEM(lic)
			if err != nil {
				return nil, err
			}

			license, err := verifier.ParseLicense(verifier.ParserOptions{
				ClusterUID: certs[0].Subject.CommonName,
				CACert:     certs[0],
				License:    lic,
			})
			if err != nil && ignoreCertificateExpiredError(err) != nil {
				return nil, err
			}

			return &licenseapi.OfflineLicense{
				ObjectMeta: metav1.ObjectMeta{
					Name:              license.PlanName,
					Namespace:         licenseSecret.Namespace,
					CreationTimestamp: *license.NotBefore,
					UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
				},
				Status: licenseapi.OfflineLicenseStatus{
					License: license,
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: licenseSecret.Name,
						},
						Key: product,
					},
				},
			}, nil
		}
	}

	return &licenseapi.OfflineLicense{}, err
}

// Lister
func (r *Storage) NewList() runtime.Object {
	return &licenseapi.OfflineLicenseList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	var licenses []licenseapi.OfflineLicense
	var err error

	list, err := listLicenseSecrets(ctx, r.kc, ns)
	for _, licenseSecret := range list {
		for product, lic := range licenseSecret.Data {
			certs, err := cert.ParseCertsPEM(lic)
			if err != nil {
				return nil, err
			}

			license, err := verifier.ParseLicense(verifier.ParserOptions{
				ClusterUID: certs[0].Subject.CommonName,
				CACert:     certs[0],
				License:    lic,
			})
			if err != nil && ignoreCertificateExpiredError(err) != nil {
				return nil, err
			}

			licenses = append(licenses, licenseapi.OfflineLicense{
				ObjectMeta: metav1.ObjectMeta{
					Name:              license.PlanName,
					Namespace:         licenseSecret.Namespace,
					CreationTimestamp: *license.NotBefore,
					UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
				},
				Status: licenseapi.OfflineLicenseStatus{
					License: license,
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: licenseSecret.Name,
						},
						Key: product,
					},
				},
			})
		}
	}

	result := licenseapi.OfflineLicenseList{
		TypeMeta: metav1.TypeMeta{},
		Items:    licenses,
	}

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func ignoreCertificateExpiredError(err error) error {
	if strings.Contains(err.Error(), "x509: certificate has expired or is not yet valid") {
		return nil
	}
	return err
}

func getLicenseSecret(ctx context.Context, kc client.Client, ns string) (*core.Secret, error) {
	var licenseSecret core.Secret
	err := kc.Get(ctx, types.NamespacedName{Name: addofflinelicense.LicenseSecretName, Namespace: ns}, &licenseSecret)
	if err != nil && kerr.IsNotFound(err) {
		return &core.Secret{}, nil // never return nil
	} else if err != nil {
		return &core.Secret{}, err // never return nil
	}
	return &licenseSecret, nil
}

func listLicenseSecrets(ctx context.Context, kc client.Client, ns string) ([]core.Secret, error) {
	var list core.SecretList
	err := kc.List(ctx, &list, client.InNamespace(ns))
	if err != nil {
		return nil, err
	}

	licenseSecrets := make([]core.Secret, 0, len(list.Items))
	for _, secret := range list.Items {
		if secret.Name == addofflinelicense.LicenseSecretName {
			licenseSecrets = append(licenseSecrets, secret)
		}
	}
	return licenseSecrets, nil
}
