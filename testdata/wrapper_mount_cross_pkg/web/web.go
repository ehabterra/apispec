// Package web is a house router wrapping chi, in its own package the way
// gitea's modules/web is, so every mount below crosses a package boundary.
package web

import (
	"encoding/json"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
)

type Router struct {
	ChiRouter *chi.Mux
	prefix    string
}

func NewRouter() *Router { return &Router{ChiRouter: chi.NewRouter()} }

func (r *Router) getPattern(p string) string { return r.prefix + p }

func (r *Router) Get(pattern string, h http.HandlerFunc) {
	r.ChiRouter.Get(r.getPattern(pattern), h)
}

// Mount forwards the pattern and the sub-router's chi field.
func (r *Router) Mount(pattern string, sub *Router) {
	r.ChiRouter.Mount(r.getPattern(pattern), sub.ChiRouter)
}

type Status struct {
	OK bool `json:"ok"`
}

func ping(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(Status{}) }

func health() *chi.Mux { m := chi.NewRouter(); m.Get("/ping", ping); return m }
func status() *chi.Mux { m := chi.NewRouter(); m.Get("/ping", ping); return m }

// MountWithHealth mounts a fixed health router on the receiver BEFORE
// forwarding the sub-router: that first mount alone cannot be derived, and must
// not hide the forwarding one.
func (r *Router) MountWithHealth(pattern string, sub *Router) {
	r.ChiRouter.Mount("/health", health())
	r.ChiRouter.Mount(pattern, sub.ChiRouter)
}

// MountWithStatus forwards first, then mounts a fixed router on the receiver.
func (r *Router) MountWithStatus(pattern string, sub *Router) {
	r.ChiRouter.Mount(pattern, sub.ChiRouter)
	r.ChiRouter.Mount("/status", status())
}

// MountFixed adds its own text to the pattern, which the call site's argument
// does not carry.
func (r *Router) MountFixed(pattern string, sub *Router) {
	r.ChiRouter.Mount(path.Join("/fixed", pattern), sub.ChiRouter)
}

// MountPair forwards the first sub-router under the caller's pattern and mounts
// the second on the receiver at a fixed path: the prefix belongs to the first
// alone.
func (r *Router) MountPair(pattern string, sub, extra *Router) {
	r.ChiRouter.Mount(pattern, sub.ChiRouter)
	r.ChiRouter.Mount("/extra", extra.ChiRouter)
}
