package http

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/unrolled/render"
)

// Redirect redirect the request
func (w *response) Redirect(location string, status ...int) {
	code := http.StatusFound
	if len(status) == 1 {
		code = status[0]
	}

	http.Redirect(w, w.req.Request(), location, code)
}

// RedirectToFirst redirects to first not empty URL
func (w *response) RedirectToFirst(appURL, appSubURL string, location ...string) {
	for _, loc := range location {
		if len(loc) == 0 {
			continue
		}

		u, err := url.Parse(loc)
		if err != nil || ((u.Scheme != "" || u.Host != "") && !strings.HasPrefix(strings.ToLower(loc), strings.ToLower(appURL))) {
			continue
		}

		w.Redirect(loc)
		return
	}
	w.Redirect(appSubURL + "/")
}

// HTMLString render content to a string but not http.ResponseWriter
func (w *response) HTMLString(name string, binding interface{}, htmlOpt ...render.HTMLOptions) (string, error) {
	var buf strings.Builder
	err := w.r.HTML(&buf, 200, name, binding, htmlOpt...)
	return buf.String(), err
}

// ServeContent serves content to http request
func (w *response) ServeContent(name string, r io.ReadSeeker, params ...interface{}) {
	modtime := time.Now()
	for _, p := range params {
		switch v := p.(type) {
		case time.Time:
			modtime = v
		}
	}
	w.Header().Set("Content-Description", "File Transfer")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Transfer-Encoding", "binary")
	w.Header().Set("Expires", "0")
	w.Header().Set("Cache-Control", "must-revalidate")
	w.Header().Set("Pragma", "public")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	http.ServeContent(w, w.req.Request(), name, modtime, r)
}

// ServeFile serves given file to response.
func (w *response) ServeFile(file string, names ...string) {
	var name string
	if len(names) > 0 {
		name = names[0]
	} else {
		name = path.Base(file)
	}
	w.Header().Set("Content-Description", "File Transfer")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Transfer-Encoding", "binary")
	w.Header().Set("Expires", "0")
	w.Header().Set("Cache-Control", "must-revalidate")
	w.Header().Set("Pragma", "public")
	http.ServeFile(w, w.req.Request(), file)
}
