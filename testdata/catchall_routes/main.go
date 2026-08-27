// Fixture: a catch-all route becomes a path template parameter, not a literal
// asterisk.
//
// OpenAPI has no wildcard. A `*` left in the path is emitted verbatim, so a
// generated client asks for the literal path `/assets*` and a validator sees a
// static path no request can match (issue #403). Each router spells the
// catch-all differently, and each spelling has to land on a template:
//
//	chi   /assets*   and   /static/*   ->   /assets/{wildcard}, /static/{wildcard}
//
// The trailing-star form is a narrowing worth knowing about: chi's `/assets*`
// also matches `/assets` with nothing after it, which OpenAPI cannot express
// either way.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type File struct {
	Name string `json:"name"`
}

func main() {
	r := chi.NewRouter()

	// A named parameter beside the catch-alls, to show the two are declared
	// the same way and only their descriptions differ.
	r.Get("/files/{id}", getFile)
	r.Get("/assets*", serveAsset)
	r.Get("/static/*", serveStatic)

	http.ListenAndServe(":8080", r)
}

func getFile(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(File{Name: chi.URLParam(r, "id")})
}

func serveAsset(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(File{Name: "asset"})
}

func serveStatic(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(File{Name: "static"})
}
