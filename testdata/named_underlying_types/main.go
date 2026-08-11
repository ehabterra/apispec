// Package main covers named types whose underlying type is not a struct
// (issue #333). Each should carry the shape of what it is defined as.
package main

import (
	"encoding/json"
	"net/http"
)

type (
	// Point is a named fixed-size array.
	Point [2]int64
	// IDs is a named slice.
	IDs []string
	// Lookup is a named map.
	Lookup map[string]int
	// Count is a named primitive.
	Count int
	// Nested is a named slice of a named array.
	Nested []Point
)

// Bundle carries one of each as a field, so both the component and the field
// rendering are covered.
type Bundle struct {
	Point  Point  `json:"point"`
	IDs    IDs    `json:"ids"`
	Lookup Lookup `json:"lookup"`
	Count  Count  `json:"count"`
	Nested Nested `json:"nested"`
}

func bundle(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Bundle{})
}

func points(w http.ResponseWriter, r *http.Request) {
	// A named array returned on its own, not as a field.
	_ = json.NewEncoder(w).Encode(Point{1, 2})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /bundle", bundle)
	mux.HandleFunc("GET /points", points)
	_ = http.ListenAndServe(":8080", mux)
}
