// Package htmlrender renders HTML pages through an interface held by the context.
package htmlrender

import (
	"html/template"
	"io"
	"net/http"
)

type Renderer interface {
	HTML(w io.Writer, status int, name string, data any) error
}

type pageRenderer struct {
	t *template.Template
}

func (r *pageRenderer) HTML(w io.Writer, status int, name string, data any) error {
	if resp, ok := w.(http.ResponseWriter); ok {
		if resp.Header().Get("Content-Type") == "" {
			resp.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		resp.WriteHeader(status)
	}
	return r.t.ExecuteTemplate(w, name, data)
}

func New() Renderer {
	return &pageRenderer{t: template.New("pages")}
}
