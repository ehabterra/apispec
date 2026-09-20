// Parameters are attributed to a HANDLER, and a handler can be mounted at more
// than one template. It reads the union of their names, so on each route the
// other route's names are absent from the path — and emitting them there
// produces an `in: path` parameter the template cannot bind, which OpenAPI
// forbids (`path-parameters-defined`). Issue #514.
//
// The fixture puts the shared handler next to the cases the fix must not touch:
// a handler on one route, a query parameter (not bound to the template), a
// catch-all, and a genuinely misspelled read.
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// thing is mounted at TWO templates and reads both names. On the short route
// `id` has nowhere to bind; on the long one both do.
func thing(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	_ = chi.URLParam(r, "codeId")
	w.WriteHeader(http.StatusOK)
}

// single is the ordinary case: one route, one name, nothing to drop.
func single(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "userId")
	w.WriteHeader(http.StatusOK)
}

// withQuery reads a path name belonging to its sibling AND a query parameter.
// The query parameter is not bound to the template and must survive on both.
func withQuery(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "orgId")
	_ = r.URL.Query().Get("page")
	w.WriteHeader(http.StatusOK)
}

// misspelled reads a name no route of its own declares — the typo the
// diagnostic exists for. It must not be emitted (the template cannot bind it)
// and it must still be reported.
func misspelled(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "teamID")
	w.WriteHeader(http.StatusOK)
}

// catchAll matches the remainder of the path.
func catchAll(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	r := chi.NewRouter()

	// One handler, two templates.
	r.Get("/codes/{codeId}/thing", thing)
	r.Get("/groups/{id}/codes/{codeId}/thing", thing)

	r.Get("/users/{userId}", single)

	// One handler, two templates, plus a query parameter on both.
	r.Get("/reports", withQuery)
	r.Get("/orgs/{orgId}/reports", withQuery)

	// The path declares teamId; the handler reads teamID.
	r.Get("/teams/{teamId}", misspelled)

	r.Get("/static/*", catchAll)

	_ = http.ListenAndServe(":8080", r)
}
