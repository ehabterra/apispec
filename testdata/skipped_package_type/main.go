package main

import (
	"encoding/json"
	"net/http"

	"github.com/ehabterra/apispec/testdata/skipped_package_type/broken"
	"github.com/ehabterra/apispec/testdata/skipped_package_type/twin"
)

func listRows(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(broken.Row{})
}

// The twin package has to be reachable, or it would not be loaded at all.
func listSheets(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(twin.Row{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rows", listRows)
	mux.HandleFunc("GET /sheets", listSheets)
	_ = http.ListenAndServe(":8080", mux)
}
