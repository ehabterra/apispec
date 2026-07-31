// Fixture: routes whose request bodies all decode through ONE generic helper,
// where one route's response converter READS a domain field that ANOTHER
// route's handler WRITES (issue #269).
//
// That field is the whole point. Resolving "who produced cart.Estimate" answers
// with the sibling handler that writes it, so the tracker hangs that handler's
// entire closure — including its own DecodeJSON instantiation — underneath this
// route's response converter. The sibling's body is then a well-formed request
// body candidate for the WRONG route, and last-wins lets it overwrite the right
// one. On a real service that documented four endpoints with a neighbour's DTO.
//
// The other ingredients are needed to reach that point at all: the decode helper
// is generic (one call site for every route, told apart only by its type
// argument), it lives in another package (a selector callee, golden rule #10),
// and each handler is a closure returned by a factory.
//
// The rule under test: a truncated or starved route must be documented as LESS
// detailed, never as DIFFERENTLY detailed. Every assertion allows "no request
// body" and fails only on a body belonging to another route.
package main

import (
	"net/http"

	"example.com/sharedbodyhelper/internal/api"
	"github.com/go-chi/chi/v5"
)

func main() {
	svc := &api.Service{}
	r := chi.NewRouter()
	r.Route("/api", func(g chi.Router) {
		g.Post("/alpha", api.PostAlpha(svc))
		g.Post("/bravo", api.PostBravo(svc))
		g.Post("/charlie", api.PostCharlie(svc))
		g.Post("/delta", api.PostDelta(svc))
		g.Post("/echo", api.PostEcho(svc))
		g.Post("/foxtrot", api.PostFoxtrot(svc))
	})
	_ = http.ListenAndServe(":8080", r)
}
