// Fixture: registration paths built by CONCATENATION (issue #274).
//
// This is how every oapi-codegen-generated server registers —
// `r.Post(options.BaseURL+"/things", h)` — and the whole argument used to
// resolve to nothing, so the route was documented at its mount prefix alone, or
// at "/". The literal part of the path, the only part actually written in the
// source, was the part that got lost.
//
// One route per class of operand, because each resolves by a different route:
//
//	/things       a struct field the caller left at its zero value
//	/api/health   a package-level const
//	/v2/items     a parameter the caller passed a literal for
//	{base}/dyn    an operand nothing can evaluate, which must degrade to a
//	              placeholder rather than vanish — a shortened path claims the
//	              handler answers somewhere it does not
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const apiBase = "/api"

type ThingRequest struct {
	Name string `json:"name"`
}

type ServerInterface interface {
	CreateThing(w http.ResponseWriter, r *http.Request)
}

type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

func (siw *ServerInterfaceWrapper) CreateThing(w http.ResponseWriter, r *http.Request) {
	siw.Handler.CreateThing(w, r)
}

// ChiServerOptions mirrors the options struct a generated server takes.
type ChiServerOptions struct {
	BaseURL    string
	BaseRouter chi.Router
}

type impl struct{}

func (impl) CreateThing(w http.ResponseWriter, r *http.Request) {
	var req ThingRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.WriteHeader(http.StatusCreated)
}

func health(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func listItems(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func dynamic(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// dynamicBase cannot be evaluated statically.
func dynamicBase() string { return "/" + http.MethodGet }

// HandlerWithOptions is the generated shape: the router comes from a struct
// field, the routes are registered inside a group closure, and the path is the
// options' BaseURL concatenated with the literal.
func HandlerWithOptions(si ServerInterface, options ChiServerOptions) http.Handler {
	r := options.BaseRouter
	if r == nil {
		r = chi.NewRouter()
	}
	wrapper := ServerInterfaceWrapper{Handler: si}
	r.Group(func(r chi.Router) {
		r.Post(options.BaseURL+"/things", wrapper.CreateThing)
	})
	return r
}

// registerItems takes its prefix as a parameter.
func registerItems(r chi.Router, prefix string) {
	r.Get(prefix+"/items", listItems)
}

func main() {
	r := chi.NewRouter()

	// A const operand.
	r.Get(apiBase+"/health", health)

	// A parameter operand the caller gives a literal.
	registerItems(r, "/v2")

	// An operand that cannot be evaluated.
	r.Get(dynamicBase()+"/dyn", dynamic)

	// A struct field left at its zero value by the caller.
	r.Mount("/", HandlerWithOptions(impl{}, ChiServerOptions{}))

	_ = http.ListenAndServe(":8080", r)
}
