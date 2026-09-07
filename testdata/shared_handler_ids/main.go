// Package main exercises operationId uniqueness when one handler serves many
// routes.
//
// An operationId identifies an OPERATION and must be unique across the
// document — a client generator turns it into a method name. The
// fully-qualified handler symbol cannot carry that: a shared handler or
// middleware is genuinely the resolved handler for several routes, and on a
// real project that put 1109 operations under 700 ids (issue #459).
//
// So a duplicated id is replaced, for every route holding it, by the
// operation's own method-and-path identity. A handler used once keeps its
// symbol.
package main

import (
	"encoding/json"
	"net/http"
)

// Item is the payload every route here returns.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// serveItem is registered at three different paths, so its symbol identifies
// none of them.
func serveItem(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

// listOnce is registered exactly once, so its symbol still identifies its
// operation and must be left alone.
func listOnce(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func main() {
	mux := http.NewServeMux()

	// One handler, three operations.
	mux.HandleFunc("GET /items/{id}", serveItem)
	mux.HandleFunc("GET /widgets/{id}", serveItem)
	mux.HandleFunc("POST /items", serveItem)

	// One handler, one operation.
	mux.HandleFunc("GET /catalogue", listOnce)

	_ = http.ListenAndServe(":8080", mux)
}
