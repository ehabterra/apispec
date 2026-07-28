// A house router: the project's own type in front of chi, the shape gitea uses in
// modules/web/router.go.
//
// Every verb method delegates to one shared registrar, and the group prefix lives
// in a FIELD that getPattern applies — there is no sub-router to mount, so the
// prefix is only knowable by following the field.
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Router struct {
	chiRouter      *chi.Mux
	curGroupPrefix string
}

func NewRouter() *Router {
	return &Router{chiRouter: chi.NewRouter()}
}

func (r *Router) getPattern(pattern string) string {
	if pattern == "" {
		return r.curGroupPrefix
	}
	return r.curGroupPrefix + pattern
}

// Methods is the one registrar every verb goes through. The verb arrives as an
// ARGUMENT, and may name several: gitea registers "GET,POST" in one call.
func (r *Router) Methods(methods, pattern string, h ...any) {
	if len(h) == 0 {
		return
	}
	var handlerFunc http.HandlerFunc
	switch fn := h[len(h)-1].(type) {
	case func(http.ResponseWriter, *http.Request):
		handlerFunc = fn
	case http.HandlerFunc:
		handlerFunc = fn
	default:
		return
	}
	r.chiRouter.Method(methods, r.getPattern(pattern), handlerFunc)
}

func (r *Router) Get(pattern string, h ...any)    { r.Methods("GET", pattern, h...) }
func (r *Router) Post(pattern string, h ...any)   { r.Methods("POST", pattern, h...) }
func (r *Router) Put(pattern string, h ...any)    { r.Methods("PUT", pattern, h...) }
func (r *Router) Delete(pattern string, h ...any) { r.Methods("DELETE", pattern, h...) }

// Group accumulates the prefix rather than mounting a sub-router.
func (r *Router) Group(pattern string, fn func()) {
	prev := r.curGroupPrefix
	r.curGroupPrefix += pattern
	fn()
	r.curGroupPrefix = prev
}
