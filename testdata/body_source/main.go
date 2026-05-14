// Package main exercises the request-body-source resolver.
//
// Two routes share the same Decode pattern but differ in *source*:
//
//   - POST /create reads json.NewDecoder(r.Body).Decode(&req) — a real
//     request body. APISpec must record a requestBody for this route.
//
//   - GET  /sync   reads json.NewDecoder(remote.Body).Decode(&payload)
//     from the response of an outbound http.Get. This is NOT a request body
//     and APISpec must not classify it as one.
//
// A third route (POST /refresh) uses an inline json.Unmarshal on a []byte
// read from another file. Its source is a file, not a request, so the
// SyncPayload schema must not appear as a request body either.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SyncPayload struct {
	UpstreamID string `json:"upstream_id"`
}

type RefreshConfig struct {
	Token string `json:"token"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/create", createUser)
	mux.HandleFunc("/sync", syncFromUpstream)
	mux.HandleFunc("/refresh", refresh)
	_ = http.ListenAndServe(":8080", mux)
}

// createUser MUST be detected as having a request body.
func createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// syncFromUpstream MUST NOT be detected as having a request body. The
// decoder reads from an outbound HTTP response, not from r.Body.
func syncFromUpstream(w http.ResponseWriter, r *http.Request) {
	remote, err := http.Get("https://example.com/sync")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer remote.Body.Close()

	var payload SyncPayload
	if err := json.NewDecoder(remote.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = payload
	w.WriteHeader(http.StatusOK)
}

// refresh MUST NOT be detected as having a request body. The bytes come
// from a file on disk, not from r.Body.
func refresh(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open("/etc/refresh.json")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cfg RefreshConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = cfg
	w.WriteHeader(http.StatusOK)
}
