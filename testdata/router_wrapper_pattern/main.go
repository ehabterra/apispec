// Package main reproduces the shape every project that wraps its router is
// written in: a wrapper takes the path as a PARAMETER, composes it into a
// LOCAL, and hands that local to the real framework registration.
//
//	func (r *Router) Methods(pattern string, h ...any) {
//		full := r.getPattern(pattern)
//		r.With(r.mw...).register(full, handler)
//	}
//
// The extractor re-extracts into the same RouteInfo from a route node's
// children — that is how a chain-style route resolves, where the outer
// `Methods("GET")` carries no path and `.Path("/x")` supplies it. A wrapper
// descends into the same walk, and what it reaches is the SAME route one hop
// further from the literal, whose path operand is `full`. A local assigned from
// a CALL resolves to nothing, so it became `{full}` — and that placeholder
// replaced the caller's own literal, dropping the route.
//
// THE CHAINED CALL IS THE WHOLE POINT. Both of these resolve correctly and are
// kept here as controls:
//
//	r.mux.HandleFunc(full, handler)   // registers directly
//	r.register(full, handler)         // one indirection, same receiver
//
// Only `r.With(...).register(full, handler)` breaks it, because the receiver is
// a new value returned by another call in the same expression. A fixture
// carrying just the first two reports the bug as already fixed.
//
// Taken from gitea's modules/web.Router (issue #494), where it cost 30 paths
// and invented 9 more — `/.well-known/{fullPattern}`,
// `/repos/{username}/{reponame}/{fullPattern}` — endpoints that do not exist,
// named after the wrapper's own local.
package main

import "net/http"

type Router struct {
	mux    *http.ServeMux
	prefix string
	mw     []func(http.Handler) http.Handler
}

// With returns a router carrying extra middleware, so the registration below
// hangs off a CHAINED call rather than off the wrapper directly. gitea's is
// `r.chiRouter.With(middlewares...).Method(m, fullPattern, handlerFunc)`.
func (r *Router) With(mw ...func(http.Handler) http.Handler) *Router {
	return &Router{mux: r.mux, prefix: r.prefix, mw: append(r.mw, mw...)}
}

func (r *Router) register(pattern string, h http.HandlerFunc) {
	r.mux.HandleFunc(pattern, h)
}

func (r *Router) getPattern(pattern string) string {
	return r.prefix + pattern
}

// Methods is the wrapper: the path arrives as a parameter and reaches the
// registration through a local.
func (r *Router) Methods(pattern string, h ...any) {
	handler := wrapHandler(h)
	full := r.getPattern(pattern)
	r.With(r.mw...).register(full, handler)
}

// wrapHandler is the variadic-handler unwrap every wrapper has.
func wrapHandler(h []any) http.HandlerFunc {
	if len(h) == 0 {
		return nil
	}
	f, _ := h[0].(http.HandlerFunc)
	return f
}

// Direct registers without the local, to keep the two apart: if this one also
// broke, the gap would be the parameter and not the local.
func (r *Router) Direct(pattern string, h http.HandlerFunc) {
	r.mux.HandleFunc(pattern, h)
}

func main() {
	r := &Router{mux: http.NewServeMux()}
	// SEVERAL callers with DIFFERENT literals. With one caller the parameter
	// resolves from the call sites alone ("they all pass the same thing"), which
	// is not the shape that breaks — a real wrapper has hundreds of callers and
	// no single answer, so the path can only come from the tree.
	r.Methods("/wrapped", wrappedHandler)
	r.Methods("/wrapped-two", wrappedHandler)
	r.Methods("/wrapped-three", wrappedHandler)
	r.Direct("/direct", directHandler)
	r.Direct("/direct-two", directHandler)
	r.mux.HandleFunc("/plain", plainHandler)
	_ = http.ListenAndServe(":8080", r.mux)
}

// wrappedHandler is reached through the wrapper's local.
func wrappedHandler(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

// directHandler is reached through the wrapper's parameter only.
func directHandler(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

// plainHandler is registered with no wrapper at all.
func plainHandler(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }
