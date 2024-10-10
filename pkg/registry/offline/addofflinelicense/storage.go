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

package addofflinelicense

import (
	"context"
	"errors"
	"strings"

	licenseapi "kubeops.dev/ui-server/apis/offline/v1alpha1"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
	kutil "kmodules.xyz/client-go"
	cg "kmodules.xyz/client-go/client"
	"kmodules.xyz/client-go/meta"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LicenseSecretName = "license-proxyserver-licenses"
)

var secretGR = schema.GroupResource{
	Group:    "",
	Resource: "secrets",
}

type Storage struct {
	kc client.Client
	a  authorizer.Authorizer
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc: kc,
		a:  a,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return licenseapi.SchemeGroupVersion.WithKind(licenseapi.ResourceKindAddOfflineLicense)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(licenseapi.ResourceKindAddOfflineLicense)
}

func (r *Storage) New() runtime.Object {
	return &licenseapi.AddOfflineLicense{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*licenseapi.AddOfflineLicense)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	if req.Namespace == "" {
		return nil, apierrors.NewBadRequest("missing license secret namespace")
	}
	if req.License == "" {
		return nil, apierrors.NewBadRequest("missing license info")
	}

	var vt kutil.VerbType
	defer func() {
		if vt != kutil.VerbUnchanged {
			// restart license-proxyserver pods
			e2 := r.kc.DeleteAllOf(ctx, &core.Pod{}, client.InNamespace(meta.PodNamespace()), client.MatchingLabels{
				"app.kubernetes.io/instance": "license-proxyserver",
				"app.kubernetes.io/name":     "license-proxyserver",
			})
			klog.InfoS("restarted license-proxyserver pods", "err", e2)
		}
	}()

	var licenseSecret core.Secret
	err := r.kc.Get(ctx, types.NamespacedName{Name: LicenseSecretName, Namespace: req.Namespace}, &licenseSecret)
	if err != nil && apierrors.IsNotFound(err) {
		// check permission
		attrs := authorizer.AttributesRecord{
			User:            user,
			Verb:            "create",
			Namespace:       req.Namespace,
			APIGroup:        secretGR.Group,
			Resource:        secretGR.Resource,
			Name:            LicenseSecretName,
			ResourceRequest: true,
		}
		decision, why, err := r.a.Authorize(ctx, attrs)
		if err != nil {
			return nil, apierrors.NewInternalError(err)
		}
		if decision != authorizer.DecisionAllow {
			return nil, apierrors.NewForbidden(secretGR, LicenseSecretName, errors.New(why))
		}

		productKey, err := getProductKey([]byte(req.License))
		if err != nil {
			return nil, err
		}

		licenseSecret = core.Secret{
			ObjectMeta: controllerruntime.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: req.Namespace,
			},
			Data: map[string][]byte{
				productKey: []byte(req.License),
			},
		}
		if err = r.kc.Create(ctx, &licenseSecret); err != nil {
			return nil, err
		}
		vt = kutil.VerbCreated

		in.Response = &licenseapi.AddOfflineLicenseResponse{
			SecretKeyRef: &core.SecretKeySelector{
				LocalObjectReference: core.LocalObjectReference{
					Name: licenseSecret.Name,
				},
				Key: productKey,
			},
		}
		return in, nil
	} else if err != nil {
		return nil, err
	}

	// check permission
	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "patch",
		Namespace:       req.Namespace,
		APIGroup:        secretGR.Group,
		Resource:        secretGR.Resource,
		Name:            LicenseSecretName,
		ResourceRequest: true,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return nil, apierrors.NewForbidden(secretGR, LicenseSecretName, errors.New(why))
	}

	productKey, err := getProductKey([]byte(req.License))
	if err != nil {
		return nil, err
	}

	vt, err = cg.CreateOrPatch(ctx, r.kc, &licenseSecret, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*core.Secret)
		if in.Data == nil {
			in.Data = map[string][]byte{}
		}
		in.Data[productKey] = []byte(req.License)
		return in
	})
	if err != nil {
		return nil, err
	}

	in.Response = &licenseapi.AddOfflineLicenseResponse{
		SecretKeyRef: &core.SecretKeySelector{
			LocalObjectReference: core.LocalObjectReference{
				Name: licenseSecret.Name,
			},
			Key: productKey,
		},
	}
	return in, nil
}

func getProductKey(lic []byte) (string, error) {
	certs, err := cert.ParseCertsPEM(lic)
	if err != nil {
		return "", err
	}
	return certs[0].Subject.OrganizationalUnit[0], nil
}
