// Package main wires two sibling packages whose import paths share a byte
// prefix but not a path-segment prefix: ".../module_prefix_packages/api" and
// ".../module_prefix_packages/app" have the common byte prefix ".../ap".
//
// The dependency analyser used to infer the project root as the byte-wise
// longest common prefix of the loaded package paths, then use that string as a
// HasPrefix membership test (issue #282). Both packages' routes must be
// documented; the module path from go.mod is what decides they are ours.
package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/module_prefix_packages/api"
	"github.com/ehabterra/apispec/testdata/module_prefix_packages/app"
	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	api.Register(r)
	app.Register(r)
	_ = http.ListenAndServe(":8080", r)
}
