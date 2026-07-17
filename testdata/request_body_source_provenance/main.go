// Fixture: request-body source gating by PROVENANCE, moved to the per-route
// extraction step (the read-side mirror of issue #170). A generic decoder
// (json.NewDecoder(x).Decode(v)) is a request body only when its source x
// traces — through parameters and assignments — back to the request's body
// (r.Body). A decoder wrapped in a `func decodeFrom(src io.Reader, v)` helper
// called with r.Body is recognised (no false negative); the same helper called
// with a bytes.Buffer / strings.Reader is not a request body (no false
// positive), because the source is resolved per-route.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Config is decoded from non-request readers only — never a request body.
type Config struct {
	Debug bool `json:"debug"`
}

// decodeFrom is a generic helper whose source is an io.Reader parameter.
func decodeFrom(src io.Reader, v any) error {
	return json.NewDecoder(src).Decode(v)
}

// --- KEEP: the source traces to r.Body ---

// createDirect decodes inline from r.Body.
func createDirect(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = req
}

// createViaHelper decodes through the io.Reader helper — source traces to r.Body.
func createViaHelper(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = decodeFrom(r.Body, &req)
	_ = req
}

// createViaAssign decodes through a local aliased to r.Body.
func createViaAssign(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	src := r.Body
	_ = json.NewDecoder(src).Decode(&req)
	_ = req
}

// --- DROP: the source is not the request ---

// decodeBuffer decodes a Config from a buffer through the SAME helper — must NOT
// be a request body (per-route resolution distinguishes it from createViaHelper).
func decodeBuffer(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	buf := bytes.NewBufferString("{}")
	_ = decodeFrom(buf, &cfg)
	_ = cfg
}

// decodeString decodes a Config from a strings.Reader — not a request body.
func decodeString(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	_ = json.NewDecoder(strings.NewReader("{}")).Decode(&cfg)
	_ = cfg
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /direct", createDirect)
	mux.HandleFunc("POST /helper", createViaHelper)
	mux.HandleFunc("POST /assign", createViaAssign)
	mux.HandleFunc("POST /decode-buffer", decodeBuffer)
	mux.HandleFunc("POST /decode-string", decodeString)
	_ = http.ListenAndServe(":8080", mux)
}
