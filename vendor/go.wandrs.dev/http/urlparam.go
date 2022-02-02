// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2020 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package http

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Params returns the param on route
func (r *request) Params(p string) string {
	s, _ := url.PathUnescape(chi.URLParam(r.req, strings.TrimPrefix(p, ":")))
	return s
}

// ParamsInt returns the param on route as int
func (r *request) ParamsInt(p string) int {
	v, _ := strconv.ParseInt(r.Params(p), 10, 0)
	return int(v)
}

// ParamsInt64 returns the param on route as int64
func (r *request) ParamsInt64(p string) int64 {
	v, _ := strconv.ParseInt(r.Params(p), 10, 64)
	return v
}

// ParamsFloat64 returns the param on route as float64
func (r *request) ParamsFloat64(name string) float64 {
	v, _ := strconv.ParseFloat(r.Params(name), 64)
	return v
}

// SetParams set params into routes
func (r *request) SetParams(k, v string) {
	chiCtx := chi.RouteContext(r.req.Context())
	chiCtx.URLParams.Add(strings.TrimPrefix(k, ":"), url.PathEscape(v))
}
