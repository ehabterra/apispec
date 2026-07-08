// Package main exercises gorilla/mux path-parameter wiring beyond the direct
// `vars := mux.Vars(r); vars["id"]` idiom:
//
//   - regex-constrained params `{sku:[a-z0-9-]+}` (OpenAPI path must drop the
//     regex to `{sku}` and surface it as a schema pattern);
//   - helper indirection, where the handler reads the var through a wrapper
//     (`pathVar(r, "id")`) that itself calls mux.Vars — resolved via call-graph
//     reachability, not the tracker subtree;
//   - a placeholder the handler never reads, which must stay warned.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

type Product struct {
	SKU  string `json:"sku"`
	Name string `json:"name"`
}

// pathVar wraps mux.Vars behind a helper with a parameter key — the indirection
// that call-graph reachability must see through.
func pathVar(r *http.Request, key string) string {
	return mux.Vars(r)[key]
}

// getProduct reads a regex-constrained param directly.
func getProduct(w http.ResponseWriter, r *http.Request) {
	sku := mux.Vars(r)["sku"]
	_ = json.NewEncoder(w).Encode(Product{SKU: sku})
}

// getOrder reads the param through the pathVar helper.
func getOrder(w http.ResponseWriter, r *http.Request) {
	id := pathVar(r, "id")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// getItem never reads the path var, so its {id} stays warned.
func getItem(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// getTag has a key typo: the route declares {id} but the handler reads "tag" —
// so the read is always empty. Reachability alone can't catch this (the handler
// does reach mux.Vars), but recovering the actual key does: it surfaces a
// path-param mismatch diagnostic.
func getTag(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tag := vars["tag"]
	_ = json.NewEncoder(w).Encode(map[string]string{"tag": tag})
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/products/{sku:[a-z0-9-]+}", getProduct).Methods("GET")
	r.HandleFunc("/orders/{id}", getOrder).Methods("GET")
	r.HandleFunc("/items/{id}", getItem).Methods("GET")
	r.HandleFunc("/tags/{id}", getTag).Methods("GET")
	_ = http.ListenAndServe(":8080", r)
}
