// Package main exercises a handler that reaches the registration through a
// wrapper PARAMETER and a conversion.
//
// The registration's handler argument is `h`, a parameter, and rendering a
// parameter yields its TYPE — so the route was attributed to
// `net/http.HandlerFunc`, which identifies no function and costs the operation
// its responses, request body, summary and a usable operationId (issue #466).
//
// The two steps feed each other, which is why one pass of each is not enough:
// resolving the parameter can expose a conversion or wrapper CALL, and peeling
// that exposes another parameter that still has to be resolved. Here
// `Get(http.HandlerFunc(h3))` peels to `h3`, which is `registerConv`'s
// parameter, bound to `createItem` at the call in main.
//
// Only one builder chain: a registration shared by several chains collapses
// into one route before any of this is asked (issue #465), which would make a
// second chain here silently untested.
package main

import "net/http"

// Router is a house router.
type Router struct{ mux *http.ServeMux }

// Combo carries the path so several verbs can be registered from it.
type Combo struct {
	r       *Router
	pattern string
}

func (r *Router) Combo(pattern string) *Combo { return &Combo{r, pattern} }

// Get takes the handler as a parameter: at the framework call the handler is
// `h`, not the function that was passed.
func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("GET "+c.pattern, h)
	return c
}

// createItem is the concrete handler the operation must be attributed to.
func createItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

// registerConv takes the handler as ITS parameter and converts it, so the
// resolution has to alternate: parameter -> conversion -> parameter.
func registerConv(rt *Router, h3 http.HandlerFunc) {
	rt.Combo("/conv").Get(http.HandlerFunc(h3))
}

func main() {
	rt := &Router{mux: http.NewServeMux()}
	registerConv(rt, createItem)
	_ = http.ListenAndServe(":8080", rt.mux)
}
