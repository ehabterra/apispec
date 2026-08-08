// Fixture: a Mount performed inside a helper function (issue #275).
//
// A mount prefix reaches nested routes by tree containment. When the Mount is
// written one function deeper, the sub-router was built at the CALL SITE, so its
// routes hang under the caller's argument while the Mount node sits in the
// helper's body — siblings, not ancestor and descendant. The routes were still
// documented, at the wrong paths, without their prefix, which is worse than not
// documenting them: nothing in the output says the path is incomplete.
//
// Centralising mounts in a small helper is the normal shape once a codebase has
// more than a couple of modules, and it is what a DI-wired or generated server
// produces.
//
// The four mounts differ only in WHERE the Mount is written and how the prefix
// arrives, because they could fail for different reasons:
//
//	/direct    mounted at the call site — the control, which always worked
//	/api       literal prefix, inside the helper — FIXED
//	/param     prefix as a plain string parameter — STILL BROKEN
//	/named     prefix as a named string type, converted at the Mount — STILL BROKEN
//
// The last two are a DIFFERENT gap and are asserted here at their current
// (wrong) behaviour, so the test flips when they are fixed rather than the gap
// going unnoticed. Getting the sub-router's routes under the mount is what this
// fixture's fix does; resolving the PREFIX itself through a parameter is path
// resolution, and resolvePathArg does not trace parameters — note that
// resolvePathOperand already has callerArgFor for exactly this, but only for
// concatenated paths.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Thing struct {
	ID string `json:"id"`
}

// RoutePrefix is a named string type, so the Mount call takes a conversion
// rather than the parameter itself.
type RoutePrefix string

func Server() http.Handler {
	r := chi.NewRouter()
	r.Get("/things", func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(Thing{ID: "1"})
	})
	return r
}

// mountLiteral writes the prefix as a literal, inside the helper.
func mountLiteral(r *chi.Mux, server http.Handler) {
	r.Mount("/api", server)
}

// mountParam takes the prefix as a plain string parameter.
func mountParam(prefix string, r *chi.Mux, server http.Handler) {
	r.Mount(prefix, server)
}

// mountNamed takes a named type and converts it at the Mount call.
func mountNamed(prefix RoutePrefix, r *chi.Mux, server http.Handler) {
	r.Mount(string(prefix), server)
}

func main() {
	root := chi.NewRouter()
	root.Mount("/direct", Server())
	mountLiteral(root, Server())
	mountParam("/param", root, Server())
	mountNamed(RoutePrefix("/named"), root, Server())
	_ = http.ListenAndServe(":8080", root)
}
