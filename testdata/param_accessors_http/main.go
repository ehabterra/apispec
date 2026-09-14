// Package main calls every parameter accessor net/http offers, including the
// repeatable `Header.Values` that had no pattern (issue #365).
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func getItem(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	_ = r.URL.Query().Get("q")
	_ = r.URL.Query()["tag"]
	_ = r.Header.Get("X-Tenant")
	_ = r.Header.Values("X-Multi")
	_, _ = r.Cookie("session")
	_ = r.FormValue("field")
	_ = json.NewEncoder(w).Encode(Item{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", getItem)
	_ = http.ListenAndServe(":8080", mux)
}
