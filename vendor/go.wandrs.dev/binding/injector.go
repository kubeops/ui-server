package binding

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	httpw "go.wandrs.dev/http"
	"go.wandrs.dev/inject"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/unrolled/render"
)

var pool = sync.Pool{
	New: func() interface{} {
		return inject.New()
	},
}

type injectorKey struct{}

func Injector(r *render.Render) func(next http.Handler) http.Handler {
	if r == nil {
		panic("chi: render must not be nil")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Check if a routing context already exists from a parent router.
			injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
			if injector != nil {
				// in case middleware.Logger is used only for this sub router
				if ww, ok := w.(middleware.WrapResponseWriter); ok {
					injector.MapTo(ww, (*middleware.WrapResponseWriter)(nil))
				}
				// give a chance to override render.Render
				injector.MapTo(httpw.NewResponseWriter(w, req, r), (*httpw.ResponseWriter)(nil))

				next.ServeHTTP(w, req)
				return
			}

			injector = pool.Get().(inject.Injector)
			injector.Reset()

			// NOTE: req.WithContext() causes 2 allocations and context.WithValue() causes 1 allocation
			ctx := context.WithValue(req.Context(), injectorKey{}, injector)
			req = req.WithContext(ctx)

			injector.MapTo(ctx, (*context.Context)(nil))
			injector.Map(req)
			injector.MapTo(w, (*http.ResponseWriter)(nil))
			if ww, ok := w.(middleware.WrapResponseWriter); ok {
				injector.MapTo(ww, (*middleware.WrapResponseWriter)(nil))
			}
			injector.MapTo(httpw.NewResponseWriter(w, req, r), (*httpw.ResponseWriter)(nil))

			// Serve the request and once its done, put the request context back in the sync pool
			next.ServeHTTP(w, req)
			pool.Put(injector)
		})
	}
}

// Inject allows injecting new values for a given request
func Inject(fn func(inject.Injector) error) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}

			if err := fn(injector); err != nil {
				responseWriter(injector).APIError(err)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// Maps the interface{} value based on its immediate type from reflect.TypeOf.
func Map(val interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}

			injector.Map(val)
			next.ServeHTTP(w, req)
		})
	}
}

// Maps the interface{} value based on the pointer of an Interface provided.
// This is really only useful for mapping a value as an interface, as interfaces
// cannot at this time be referenced directly without a pointer.
func MapTo(val interface{}, ifacePtr interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}

			injector.MapTo(val, ifacePtr)
			next.ServeHTTP(w, req)
		})
	}
}

// Provides a possibility to directly insert a mapping based on type and value.
// This makes it possible to directly map type arguments not possible to instantiate
// with reflect like unidirectional channels.
func Set(typ reflect.Type, val reflect.Value) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
			if injector == nil {
				panic("chi: register Injector middleware")
			}

			injector.Set(typ, val)
			next.ServeHTTP(w, req)
		})
	}
}

var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

// HandlerFunc converts a regular function into a net/http handler.
//
// This regular function may have 3 possible signatures
// signature 1:
//   func doStuff(...)            # must write to http.ResponseWriter
// signature 2:
//   func doStuff(...) error      # converted to metav1.Status and written to http.ResponseWriter as a JSON object
//   func doStuff(...) []byte     # directly written using http.ResponseWriter.Write(bytes)
//   func doStuff(...) some_value # converted to JSON and written to http.ResponseWriter
// signature 3:
//   func doStuff(...) ([]byte, error)
//   func doStuff(...) (some_value, error)
//   If an error is returned, then converted to metav1.Status and written to http.ResponseWriter as a JSON object.
//   Otherwise, []byte is written directly and some_value is converted to JSON and written to http.ResponseWriter
//
// Each of these functions can take any injected values as argument including the following pre-injected ones:
//  - r *http.Request
//  - w httpw.ResponseWriter # the recommended ResponseWriter as it has helper methods like macaron.Context
//  - w http.ResponseWriter  # net/http ResponseWriter
//  - w middleware.WrapResponseWriter # go-chi's ResponseWriter wrapper. Use(middleware.Logger) to inject this.
func HandlerFunc(fn interface{}) http.HandlerFunc {
	firstReturnIsErr := false

	typ := reflect.TypeOf(fn)
	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("fn %s must be a function, found %s", typ, typ.Kind()))
	}
	switch typ.NumOut() {
	case 0:
		// nothing more to check
	case 1:
		etyp := typ.Out(0)
		if etyp.Implements(errorType) {
			firstReturnIsErr = true
		} else if reflect.New(etyp).Type().Implements(errorType) {
			panic(fmt.Sprintf("fn %s return type should be *%s to be considered an error", typ, etyp.Name()))
		}
	case 2:
		etyp := typ.Out(1)
		if !etyp.Implements(errorType) {
			if reflect.New(etyp).Type().Implements(errorType) {
				panic(fmt.Sprintf("fn %s 2nd return value should be *%s to be considered an error", typ, etyp.Name()))
			}
			panic("2nd return value must implement error")
		}
		vtyp := typ.Out(0)
		if vtyp.Implements(errorType) {
			panic(fmt.Sprintf("fn %s 1st return value must not an error", typ))
		}
	default:
		panic(fmt.Sprintf("fn %s has %d return values, at most 2 are allowed", typ, typ.NumOut()))
	}

	return func(w http.ResponseWriter, req *http.Request) {
		injector, _ := req.Context().Value(injectorKey{}).(inject.Injector)
		if injector == nil {
			panic("chi: register Injector middleware")
		}
		injector.MapTo(req.Context(), (*context.Context)(nil)) // make sure we have the latest Context

		results, err := injector.Invoke(fn)
		if err != nil {
			panic(fmt.Sprintf("failed to invoke %s, reason: %v", typ.String(), err))
		}

		ww := responseWriter(injector)
		switch len(results) {
		case 0:
			if !ww.Written() {
				panic(fmt.Sprintf("fn %s must write to ResponseWriter, since it returns nothing", typ))
			}
			return // nothing returned, assuming function directly wrote to http.ResponseWriter
		case 1:
			if firstReturnIsErr {
				err, _ := results[0].Interface().(error)
				ww.APIError(err)
				return
			}

			v := results[0]
			if isByteSlice(v) {
				_, _ = w.Write(v.Bytes())
			} else {
				ww.JSON(http.StatusOK, v.Interface())
			}
			return
		case 2:
			err, _ := results[1].Interface().(error)
			// WARNING: https://stackoverflow.com/a/46275411/244009
			if err != nil && !reflect.ValueOf(err).IsNil() /*for error wrapper interfaces*/ {
				ww.APIError(err)
				return
			}

			v := results[0]
			if isByteSlice(v) {
				_, _ = w.Write(v.Bytes())
			} else {
				ww.JSON(http.StatusOK, v.Interface())
			}
			return
		default:
			panic(fmt.Sprintf("received %d return values, can only handle upto 2 return values", len(results)))
		}
	}
}

func responseWriter(injector inject.Injector) httpw.ResponseWriter {
	return injector.GetVal(inject.InterfaceOf((*httpw.ResponseWriter)(nil))).Interface().(httpw.ResponseWriter)
}

func isByteSlice(val reflect.Value) bool {
	return val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8
}
