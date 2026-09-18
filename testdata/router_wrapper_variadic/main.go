// Package main isolates the registration shapes that issue #494's fix left
// behind, as a 2x2 matrix (issue #498).
//
// gitea's wrapper registers down two branches, and its callers pass handlers
// two ways. Both survivors and both casualties differ in BOTH respects, so the
// matrix is the only way to say which one matters:
//
//	                     handler direct   handler via append(mid, h)...
//	comma -> loop branch     /loop-direct        /loop-spread
//	single -> else branch    /single-direct      /single-spread
//
// On gitea: /favicon.ico is loop+direct and resolves, /metrics is
// single+spread and resolves, /captcha/* and /robots.txt are loop+spread and
// are NOT documented.
package main

import (
	"net/http"
	"strings"
)

type middleware = func(http.Handler) http.Handler

type Router struct {
	mux    *http.ServeMux
	prefix string
	mw     []middleware
}

// With returns a router carrying extra middleware, so the registration hangs
// off a CHAINED call — the shape #494 was about, kept here because both
// branches below go through it.
func (r *Router) With(mw ...middleware) *Router {
	return &Router{mux: r.mux, prefix: r.prefix, mw: append(r.mw, mw...)}
}

func (r *Router) register(pattern string, h http.HandlerFunc) {
	r.mux.HandleFunc(pattern, h)
}

func (r *Router) getPattern(pattern string) string { return r.prefix + pattern }

// Methods mirrors gitea's: one verb goes down the else branch, several go
// through a loop over the split list.
func (r *Router) Methods(methods, pattern string, h ...any) {
	handler := wrapHandler(h)
	full := r.getPattern(pattern)
	if strings.Contains(methods, ",") {
		for _, method := range strings.Split(methods, ",") {
			_ = strings.TrimSpace(method)
			r.With(r.mw...).register(full, handler)
		}
	} else {
		r.With(r.mw...).register(full, handler)
	}
}

// Get delegates, which is how gitea's single-verb registrations reach Methods.
func (r *Router) Get(pattern string, h ...any) { r.Methods("GET", pattern, h...) }

func wrapHandler(h []any) http.HandlerFunc {
	if len(h) == 0 {
		return nil
	}
	f, _ := h[0].(http.HandlerFunc)
	return f
}

func main() {
	r := &Router{mux: http.NewServeMux()}
	var mid []any

	r.Methods("GET,HEAD", "/loop-direct", loopDirect)
	r.Methods("GET,HEAD", "/loop-spread", append(mid, http.HandlerFunc(loopSpread))...)
	r.Methods("GET", "/single-direct", singleDirect)
	r.Get("/single-spread", append(mid, http.HandlerFunc(singleSpread))...)

	_ = http.ListenAndServe(":8080", r.mux)
}

// loopDirect is the shape gitea's /favicon.ico uses: several verbs, handler
// passed straight through.
func loopDirect(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

// loopSpread is the shape gitea's /captcha/* and /robots.txt use.
func loopSpread(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

// singleDirect is one verb, handler passed straight through.
func singleDirect(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

// singleSpread is the shape gitea's /metrics uses.
func singleSpread(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }
