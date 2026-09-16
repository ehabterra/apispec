// Package main covers every shape encoding/json gives an embedded field, since
// a struct that embeds another used to document none of what the embed carries
// (issue #487). Each case below is checked against what json.Marshal actually
// emits, not against what embedding looks like it should do.
package main

import (
	"encoding/json"
	"net/http"
)

// Base is embedded plainly, so its fields are PROMOTED — with its own tags, so
// `Kind` appears as `kind` and not as `Kind`.
type Base struct {
	ID   int
	Kind string `json:"kind"`
}

// Named is embedded WITH a tag, which nests it instead of promoting it.
type Named struct{ Label string }

// Meta is embedded through a pointer, which promotes exactly as a value does.
type Meta struct{ Page int }

// Ref is a non-struct, which contributes one field named for the type.
type Ref string

// Hidden is embedded with `json:"-"`, which drops it whole.
type Hidden struct{ Secret string }

// Item is the full set. json.Marshal gives, verbatim:
//
//	{"ID":0,"kind":"","named":{"Label":""},"Page":0,"Ref":"","Extra":""}
//
// Base promotes ID and kind, Named nests under its tag, *Meta promotes Page,
// Ref contributes one field named for the type, and Hidden contributes nothing.
type Item struct {
	Base
	Named `json:"named"`
	*Meta
	Ref
	Hidden `json:"-"`
	Extra  string
}

// Shadow pins Go's shallowest-wins rule: the outer Kind is at depth 0 and
// Base.Kind at depth 1, both named `kind` on the wire, so the OUTER one is what
// is serialised. json.Marshal gives {"ID":0,"kind":""}.
type Shadow struct {
	Base
	Kind string `json:"kind"`
}

// Deep embeds through two levels, which promotes all the way up.
type Deep struct {
	Item
	Own string
}

func item(w http.ResponseWriter, r *http.Request)   { _ = json.NewEncoder(w).Encode(Item{}) }
func shadow(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(Shadow{}) }
func deep(w http.ResponseWriter, r *http.Request)   { _ = json.NewEncoder(w).Encode(Deep{}) }

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /item", item)
	mux.HandleFunc("GET /shadow", shadow)
	mux.HandleFunc("GET /deep", deep)
	_ = http.ListenAndServe(":8080", mux)
}
