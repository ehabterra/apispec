// Package main is a router apispec has no patterns for: handlers are stored in
// a map and dispatched by hand, so there is no registration call to match.
//
// The point of the fixture is not the router — it is that analysing it produces
// an empty spec, and that apispec says so (issue #379) instead of reporting
// success over a document with no paths.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Router is a house router: routes live in a map keyed by "METHOD path", which
// no configured pattern can read.
type Router struct {
	routes map[string]http.HandlerFunc
}

func NewRouter() *Router {
	return &Router{routes: map[string]http.HandlerFunc{}}
}

func (rt *Router) Add(key string, h http.HandlerFunc) {
	rt.routes[key] = h
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := rt.routes[r.Method+" "+r.URL.Path]; ok {
		h(w, r)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func main() {
	rt := NewRouter()
	rt.Add("GET /items", listItems)
	rt.Add("POST /items", createItem)

	_ = http.ListenAndServe(":8080", rt)
}

func listItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{{ID: "1", Name: "first"}})
}

func createItem(w http.ResponseWriter, r *http.Request) {
	var in Item
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(in)
}
