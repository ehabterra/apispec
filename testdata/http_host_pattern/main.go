// Package main exercises the HOST component of a Go 1.22 ServeMux pattern.
// The grammar is "[METHOD ][HOST]/[PATH]", so the host is not part of the URL
// path and must not reach the OpenAPI path template.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func listItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func createItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

func health(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

func main() {
	mux := http.NewServeMux()

	// Method and host together, and host without a method.
	mux.HandleFunc("GET api.example.com/items", listItems)
	mux.HandleFunc("api.example.com/items/new", createItem)

	// A port is part of the host too.
	mux.HandleFunc("GET localhost:8080/debug", health)

	// Two hosts serving the SAME path. Both are the same URL to a consumer,
	// which is what the collapse below records.
	mux.HandleFunc("GET api.example.com/status", health)
	mux.HandleFunc("GET admin.example.com/status", listItems)

	// No host: must be untouched.
	mux.HandleFunc("GET /health", health)

	_ = http.ListenAndServe(":8080", mux)
}
