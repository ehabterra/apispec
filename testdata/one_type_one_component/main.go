// Package main exercises the rule that one Go type produces one component.
//
// A response type recovered from a CALL carries the package's NAME
// ("api.Issue"), while metadata's own type strings carry the import PATH
// ("twoname/api.Issue"). Both spellings used to become their own component, and
// the short-keyed one resolved WRONGLY: its qualifier matches no package key,
// so the lookup fell back to a bare-name scan over sorted packages and returned
// aaamig's function-local `type Issue struct` — one field instead of four
// (issue #457).
//
// Every route below must reference the SAME component, with all four fields.
package main

import (
	"encoding/json"
	"net/http"

	"twoname/aaamig"
	"twoname/api"
)

func mkConcrete() api.Issue { return api.Issue{} }
func mkPointer() *api.Issue { return &api.Issue{} }
func mkAny() any            { return api.Issue{} }

func respond(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func main() {
	_ = aaamig.Migrate()

	// A composite literal: the type string comes from metadata, fully qualified.
	http.HandleFunc("GET /direct", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.Issue{})
	})

	// A direct call: the return type is recovered from the call and arrives
	// qualified by the package NAME. This is the shape that broke.
	http.HandleFunc("GET /viaconcrete", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mkConcrete())
	})

	// The same through a pointer return.
	http.HandleFunc("GET /viapointer", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mkPointer())
	})

	// Assigned first, which resolves through the variable and was never broken.
	http.HandleFunc("GET /viavar", func(w http.ResponseWriter, r *http.Request) {
		v := mkConcrete()
		_ = json.NewEncoder(w).Encode(v)
	})

	// Passed to a wrapper, resolved at the wrapper's call site.
	http.HandleFunc("GET /viawrapper", func(w http.ResponseWriter, r *http.Request) {
		respond(w, api.Issue{})
	})

	// An `any`-returning helper stays unresolved: the concrete type is not
	// recoverable from the signature, and guessing one would be worse than
	// documenting none (golden rule #7).
	http.HandleFunc("GET /viaany", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mkAny())
	})

	_ = http.ListenAndServe(":8080", nil)
}
