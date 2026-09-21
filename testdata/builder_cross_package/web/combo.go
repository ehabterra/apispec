// Package web is the house router, declared where a real one lives: in another
// package from the routes that use it.
package web

import "net/http"

// Router is a thin house wrapper over ServeMux.
type Router struct{ Mux *http.ServeMux }

// Combo carries the path so several verbs can be registered from it.
type Combo struct {
	r       *Router
	pattern string
}

// NewCombo takes the path once. Every verb below reads it back off the receiver.
func (r *Router) NewCombo(pattern string) *Combo { return &Combo{r, pattern} }

// Get concatenates a literal onto the receiver field.
func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.Mux.HandleFunc("GET "+c.pattern, h)
	return c
}

// Post is a second verb, so the chain has somewhere to go.
func (c *Combo) Post(h http.HandlerFunc) *Combo {
	c.r.Mux.HandleFunc("POST "+c.pattern, h)
	return c
}
