package http

import (
	"net/http"
)

type Request interface {
	Request() *http.Request

	// url path parameters
	Params(p string) string
	ParamsInt(p string) int
	ParamsInt64(p string) int64
	ParamsFloat64(name string) float64
	SetParams(k, v string)

	// query parameters
	Query(key string, defaults ...string) string
	QueryTrim(key string, defaults ...string) string
	QueryStrings(key string, defaults ...[]string) []string
	QueryInt(key string, defaults ...int) int
	QueryInt64(key string, defaults ...int64) int64
	QueryBool(key string, defaults ...bool) bool
}

type request struct {
	req *http.Request
}

var _ Request = &request{}

func (r *request) Request() *http.Request {
	return r.req
}
