package whoami

import (
	"context"

	"kubeshield.dev/whoami/apis/identity"
	"kubeshield.dev/whoami/apis/identity/v1alpha1"

	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type Storage struct {
}

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Creater = &Storage{}

func NewStorage() *Storage {
	return &Storage{}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ResourceKindWhoAmI)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

// Getter
func (r *Storage) New() runtime.Object {
	return &identity.WhoAmI{}
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return nil, kerr.NewBadRequest("missing user info")
	}
	req := obj.(*v1alpha1.WhoAmI)

	extra := make(map[string]v1alpha1.ExtraValue)
	for k, v := range user.GetExtra() {
		extra[k] = v1alpha1.ExtraValue(v)
	}
	req.Response = &v1alpha1.WhoAmIResponse{
		User: &v1alpha1.UserInfo{
			Username: user.GetName(),
			UID:      user.GetUID(),
			Groups:   user.GetGroups(),
			Extra:    extra,
		},
	}
	return req, nil
}
