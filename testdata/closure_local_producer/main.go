// Package main registers handler CLOSURES inside a registration function, one
// of which assigns a variable that no call produced and hands it to a helper
// (issue #550).
//
// A variable assigned in a function body is linked to the call that produced
// it. With no such call, metadata used to fall back to the INVOCATION of the
// function the assignment is written in — here `registerAPI(mux)`, the route
// registration itself. The helper's argument then expanded every route
// registerAPI registers beneath it, and the redirect a sibling handler writes
// was documented as a response of /items.
package main

import (
	"encoding/json"
	"net/http"
)

// Item is what /items returns.
type Item struct {
	Name string `json:"name"`
}

// save takes a value the handler did not get from a call.
func save(name string) {}

func registerAPI(mux *http.ServeMux) {
	// A sibling handler whose only answer is a redirect.
	mux.HandleFunc("GET /login-redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path // assigned from a field read: no call produced it
		save(name)
		_ = json.NewEncoder(w).Encode(Item{Name: name})
	})
}

func main() {
	mux := http.NewServeMux()
	registerAPI(mux)
	_ = http.ListenAndServe(":8080", mux)
}
