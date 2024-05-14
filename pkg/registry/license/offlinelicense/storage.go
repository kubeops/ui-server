package offlinelicense

import (
	"context"
	"strings"

	licenseapi "kubeops.dev/ui-server/apis/offline/v1alpha1"

	licstatus "go.bytebuilders.dev/license-proxyserver/apis/proxyserver/v1alpha1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
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

// Lister
func (r *Storage) NewList() runtime.Object {
	return &licenseapi.OfflineLicenseList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	var licenses []licenseapi.OfflineLicense
	var err error

	var licStatusList licstatus.LicenseStatusList
	err = r.kc.List(ctx, &licStatusList)
	if err != nil && kerr.IsNotFound(err) {
		return &licenseapi.OfflineLicenseList{}, nil
	} else if err != nil {
		return nil, err
	}

	for _, lic := range licStatusList.Items {
		licenses = append(licenses, licenseapi.OfflineLicense{
			ObjectMeta: metav1.ObjectMeta{
				Name: lic.Status.License.PlanName,
			},
			Status: licenseapi.OfflineLicenseStatus{
				License: lic.Status.License,
			},
		})
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
