package main

import (
	"net/http"

	// This would normally cause CGO errors
	_ "github.com/davidbyttow/govips/v2/vips"
	_ "github.com/wamuir/graft/tensorflow"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "success"}`))
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/status", apiHandler)
	http.ListenAndServe(":8080", nil)
}
