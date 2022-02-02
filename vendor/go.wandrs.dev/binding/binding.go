// Copyright 2014 Martini Authors
// Copyright 2014 The Macaron Authors
// Copyright 2020 The Gitea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Package binding is a middleware that provides request data binding and validation for Chi.
package binding

import (
	"io"
	"net/http"
	"reflect"
	"strings"

	"go.wandrs.dev/inject"

	"github.com/go-playground/form/v4"
	"github.com/go-playground/validator/v10"
	jsoniter "github.com/json-iterator/go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var validate = validator.New()

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Bind wraps up the functionality of the Form and Json middleware
// according to the Content-Type and verb of the request.
// A Content-Type is required for POST and PUT requests.
// Bind invokes the ErrorHandler middleware to bail out if errors
// occurred. If you want to perform your own error handling, use
// Form or Json middleware directly. An interface pointer can
// be added as a second argument in order to map the struct to
// a specific interface.
func Bind(obj interface{}, ifacePtr ...interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			injector, _ := r.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}

			var err *apierrors.StatusError
			contentType := r.Header.Get("Content-Type")
			if r.Method == http.MethodPost || r.Method == http.MethodPut || len(contentType) > 0 {
				switch {
				case strings.Contains(contentType, "form-urlencoded"):
					err = bindForm(r, injector, obj, ifacePtr...)
				case strings.Contains(contentType, "multipart/form-data"):
					err = bindMultipartForm(r, injector, obj, ifacePtr...)
				case strings.Contains(contentType, "json"):
					err = bindJSON(r, injector, obj, ifacePtr...)
				default:
					status := metav1.Status{
						Status: metav1.StatusFailure,
						Code:   http.StatusUnsupportedMediaType,
						Reason: metav1.StatusReasonUnsupportedMediaType,
					}
					if contentType == "" {
						status.Message = "Empty Content-Type"
					} else {
						status.Message = "Unsupported Content-Type"
					}
					err = &apierrors.StatusError{status}
				}
			} else {
				err = bindForm(r, injector, obj, ifacePtr...)
			}

			if err != nil {
				ww := responseWriter(injector)
				ww.APIError(err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Form is middleware to deserialize form-urlencoded data from the request.
// It gets data from the form-urlencoded body, if present, or from the
// query string. It uses the http.Request.ParseForm() method
// to perform deserialization, then reflection is used to map each field
// into the struct with the proper type. Structs with primitive slice types
// (bool, float, int, string) can support deserialization of repeated form
// keys, for example: key=val1&key=val2&key=val3
// An interface pointer can be added as a second argument in order
// to map the struct to a specific interface.
func Form(obj interface{}, ifacePtr ...interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			injector, _ := r.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}
			if err := bindForm(r, injector, obj, ifacePtr...); err != nil {
				ww := responseWriter(injector)
				ww.APIError(err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bindForm(r *http.Request, injector inject.Injector, obj interface{}, ifacePtr ...interface{}) *apierrors.StatusError {
	ensureNotPointer(obj)
	newObj := reflect.New(reflect.TypeOf(obj))

	if err := r.ParseForm(); err != nil {
		return apierrors.NewBadRequest(err.Error())
	}

	d := form.NewDecoder()
	if err := d.Decode(newObj.Interface(), r.Form); err != nil {
		return NewBindingError(err, obj)
	}

	if err := check(newObj); err != nil {
		return NewBindingError(err, obj)
	}

	injector.Map(newObj.Elem().Interface())
	if len(ifacePtr) > 0 {
		injector.MapTo(newObj.Elem().Interface(), ifacePtr[0])
	}
	return nil
}

// MaxMemory represents maximum amount of memory to use when parsing a multipart form.
// Set this to whatever value you prefer; default is 10 MB.
var MaxMemory = int64(1024 * 1024 * 10)

// MultipartForm works much like Form, except it can parse multipart forms
// and handle file uploads. Like the other deserialization middleware handlers,
// you can pass in an interface to make the interface available for injection
// into other handlers later.
func MultipartForm(obj interface{}, ifacePtr ...interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			injector, _ := r.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}
			if err := bindMultipartForm(r, injector, obj, ifacePtr...); err != nil {
				ww := responseWriter(injector)
				ww.APIError(err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bindMultipartForm(r *http.Request, injector inject.Injector, obj interface{}, ifacePtr ...interface{}) *apierrors.StatusError {
	ensureNotPointer(obj)

	newObj := reflect.New(reflect.TypeOf(obj))
	// This if check is necessary due to https://github.com/martini-contrib/csrf/issues/6
	if r.Form == nil {
		if err := r.ParseMultipartForm(MaxMemory); err != nil {
			return apierrors.NewBadRequest(err.Error())
		}
	}

	d := form.NewDecoder()
	if err := d.Decode(newObj.Interface(), r.Form); err != nil {
		return NewBindingError(err, obj)
	}

	if err := check(newObj); err != nil {
		return NewBindingError(err, obj)
	}

	injector.Map(newObj.Elem().Interface())
	if len(ifacePtr) > 0 {
		injector.MapTo(newObj.Elem().Interface(), ifacePtr[0])
	}
	return nil
}

// JSON is middleware to deserialize a JSON payload from the request
// into the struct that is passed in. The resulting struct is then
// validated, but no error handling is actually performed here.
// An interface pointer can be added as a second argument in order
// to map the struct to a specific interface.
//
// For all requests, Json parses the raw query from the URL using matching struct json tags.
//
// For POST, PUT, and PATCH requests, it also parses the request body.
// Request body parameters take precedence over URL query string values.
//
// Json follows the Request.ParseForm() method from Go's net/http library.
// ref: https://github.com/golang/go/blob/700e969d5b23732179ea86cfe67e8d1a0a1cc10a/src/net/http/request.go#L1176
func JSON(obj interface{}, ifacePtr ...interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			injector, _ := r.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}
			if err := bindJSON(r, injector, obj, ifacePtr...); err != nil {
				ww := responseWriter(injector)
				ww.APIError(err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bindJSON(r *http.Request, injector inject.Injector, obj interface{}, ifacePtr ...interface{}) *apierrors.StatusError {
	ensureNotPointer(obj)
	newObj := reflect.New(reflect.TypeOf(obj))

	if r.URL != nil {
		if params := r.URL.Query(); len(params) > 0 {
			d := form.NewDecoder()
			d.SetTagName("json")
			if err := d.Decode(newObj.Interface(), params); err != nil {
				return NewBindingError(err, obj)
			}
		}
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(newObj.Interface()); err != nil && err != io.EOF {
				return apierrors.NewBadRequest(err.Error())
			}
		}
	}

	if err := check(newObj); err != nil {
		return NewBindingError(err, obj)
	}

	injector.Map(newObj.Elem().Interface())
	if len(ifacePtr) > 0 {
		injector.MapTo(newObj.Elem().Interface(), ifacePtr[0])
	}
	return nil
}

// Don't pass in pointers to bind to. Can lead to bugs.
func ensureNotPointer(obj interface{}) {
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		panic("Pointers are not accepted as binding models")
	}
}

func check(val reflect.Value) error {
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		val = val.Elem()
	}

	if val.Kind() == reflect.Struct {
		return validate.Struct(val.Interface())
	} else if val.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			if err := validate.Struct(val.Index(i).Interface()); err != nil {
				return err
			}
		}
	}
	return nil
}
