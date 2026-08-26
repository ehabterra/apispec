// Fixture: a wrapper whose generic payload is nil must not be "specialised".
//
// A response helper that wraps every payload in an envelope is specialised by
// composing an allOf: the envelope's $ref plus the concrete type of the field
// the caller bound. When the caller passes no payload at all — `NewEnvelope(nil)`,
// the shape of every logout and delete endpoint — there is nothing to
// specialise, and the response is just the envelope.
//
// The override for such a field used to resolve to a nil schema and be skipped.
// Once an unmappable type resolves to the empty schema instead (issue #395),
// the override fired with a schema that constrains nothing, turning
// `{$ref: Envelope}` into `allOf: [{$ref: Envelope}, {properties: {data: {}}}]`
// — the same claim with noise around it. Measured on a 163-route service, it
// hit 27 responses.
package main

import (
	"encoding/json"
	"net/http"
)

type Envelope struct {
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewEnvelope binds its parameter to the Data field, which is what makes the
// field specialisable from the call site.
func NewEnvelope(data any) Envelope {
	return Envelope{Message: "ok", Data: data}
}

// respondJSON is the shared writer: it builds the envelope from whatever the
// caller passed, which is what makes Data specialisable per call site.
func respondJSON(w http.ResponseWriter, data any) {
	response := NewEnvelope(data)
	_ = json.NewEncoder(w).Encode(response)
}

// logout carries no payload: the response is the envelope itself.
func logout(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, nil)
}

// profile carries one: the envelope specialised with the concrete type, which
// must keep working.
func profile(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, User{ID: "1", Name: "ada"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /logout", logout)
	mux.HandleFunc("GET /profile", profile)
	http.ListenAndServe(":8080", mux)
}
