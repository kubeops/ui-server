package binding

import (
	gojson "encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/go-playground/form/v4"
	"github.com/go-playground/validator/v10"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewBindingError returns an error indicating the request is invalid and cannot be bound to an object.
func NewBindingError(err error, obj interface{}) *apierrors.StatusError {
	if err == nil {
		return &apierrors.StatusError{metav1.Status{
			Status: metav1.StatusSuccess,
			Code:   http.StatusNoContent,
		}}
	}

	switch t := err.(type) {
	case *validator.InvalidValidationError:
		return &apierrors.StatusError{metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			//Details: &metav1.StatusDetails{
			//	Group:  qualifiedKind.Group,
			//	Kind:   qualifiedKind.Kind,
			//	Name:   name,
			//	Causes: causes,
			//},
			Message: err.Error(),
		}}
	case validator.ValidationErrors:
		causes := make([]metav1.StatusCause, 0, len(t))
		for i := range t {
			err := t[i]
			st := metav1.CauseTypeFieldValueInvalid
			if err.Tag() == "required" {
				st = metav1.CauseTypeFieldValueRequired
			}
			causes = append(causes, metav1.StatusCause{
				Type:    st,
				Message: err.Error(),
				Field:   err.Namespace(),
			})
		}
		return &apierrors.StatusError{metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			Details: &metav1.StatusDetails{
				//Group:  qualifiedKind.Group,
				//Kind:   qualifiedKind.Kind,
				//Name:   name,
				Causes: causes,
			},
			// Message: fmt.Sprintf("%s %q is invalid: %v", qualifiedKind.String(), name, errs.ToAggregate()),
			Message: fmt.Sprintf("%s is invalid", reflect.TypeOf(obj)),
		}}
	case form.DecodeErrors:
		ot := reflect.TypeOf(obj)
		if ot.Kind() == reflect.Interface {
			ot = ot.Elem()
		}
		causes := make([]metav1.StatusCause, 0, len(t))
		for field, err := range t {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: err.Error(),
				Field:   field,
			})
		}
		return &apierrors.StatusError{metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusBadRequest,
			Reason: metav1.StatusReasonBadRequest,
			Details: &metav1.StatusDetails{
				// Group:  qualifiedKind.Group,
				//Kind:   qualifiedKind.Kind,
				//Name:   name,
				Causes: causes,
			},
			Message: fmt.Sprintf("failed to decode into %s", reflect.TypeOf(obj)),
		}}
	case *form.InvalidDecoderError, *gojson.InvalidUnmarshalError:
		return apierrors.NewInternalError(err) // error due to bug in source code
	default:
		return apierrors.NewBadRequest(err.Error()) // error due to bad input from request body
	}
}
