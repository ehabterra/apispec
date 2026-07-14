package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// User is the API resource served by the chi router.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// VersionInfo is served by the plain net/http ops endpoint.
type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	// A raw net/http header read inside a chi handler: the merged stdlib
	// param patterns must document it even though the route is chi's.
	_ = r.Header.Get("X-Request-ID")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode([]User{})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	_ = json.NewDecoder(r.Body).Decode(&u)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(u)
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(VersionInfo{Version: "1.0.0"})
}

// statusHandler dispatches on r.Method: the verb-less plain registration
// must split per served verb, not default to a single POST.
func statusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	// The API runs on chi …
	r := chi.NewRouter()
	r.Get("/api/users", listUsers)
	r.Post("/api/users", createUser)

	// … while ops endpoints are registered on the plain default ServeMux,
	// the way expvar/pprof-style wiring usually is.
	http.HandleFunc("GET /ops/version", versionHandler)
	http.HandleFunc("/ops/status", statusHandler)

	go func() { _ = http.ListenAndServe(":9090", nil) }()
	_ = http.ListenAndServe(":8080", r)
}
