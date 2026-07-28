// Fixture: a closure has no declared name, so apispec identifies it by WHERE it
// is written — and that identity becomes the route's operationId.
//
// While the position carried the absolute file path, the same source produced a
// different spec on every machine: an operationId read
// `FuncLit:/Users/someone/checkout/main.go:31:24`, so the output could not be
// diffed against a colleague's, reviewed in a PR, or committed. Seen on
// photoprism, where 116 of 131 operations were named that way.
//
// Two closure routes, because one would not show whether the identity stays
// unique once the path is dropped.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Item{})
	})

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var in Item
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.WriteHeader(http.StatusCreated)
	})

	_ = http.ListenAndServe(":8080", mux)
}
