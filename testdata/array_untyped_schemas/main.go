// Package main covers two type shapes that are not nameable as components:
// fixed-size arrays and untyped constants (issue #326).
package main

import (
	"encoding/json"
	"net/http"
)

// Point is gitea's heatmap shape: a slice of fixed-size arrays.
type Point [2]int64

// Series carries both a named fixed-size array and an anonymous one, plus the
// [][]T shape #259 fixed, so all three stay covered together.
type Series struct {
	Points [][2]int64 `json:"points"`
	Named  []Point    `json:"named"`
	Fixed  [3]string  `json:"fixed"`
	Nested [][]int    `json:"nested"`
	Labels []string   `json:"labels"`
}

func series(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Series{})
}

// heatmap is gitea's shape verbatim: a local variable of slice-of-fixed-array
// type, encoded directly. The type reaches the mapper through the argument, not
// through a field declaration.
func heatmap(w http.ResponseWriter, r *http.Request) {
	data := make([][2]int64, 0, 4)
	data = append(data, [2]int64{1, 2})
	_ = json.NewEncoder(w).Encode(data)
}

func status(w http.ResponseWriter, r *http.Request) {
	// Untyped constants: go/types renders these as "untyped bool" / "untyped
	// int" / "untyped string", which are not primitives by name.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"count": 42,
		"label": "ready",
	})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /series", series)
	mux.HandleFunc("GET /heatmap", heatmap)
	mux.HandleFunc("GET /status", status)
	_ = http.ListenAndServe(":8080", mux)
}
