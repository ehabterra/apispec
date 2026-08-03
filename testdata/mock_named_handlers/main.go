// Package main exercises production declarations whose *names* collide with
// test-double vocabulary ("mock", "fake", "stub"). Metadata used to drop any
// such declaration — and every call made from it — by substring-matching the
// identifier, which erased the bodies of perfectly ordinary endpoints (a
// fake-door A/B test, a sandbox tenant serving canned data). Each handler here
// has a plainly-named twin: the two must document the same schema.
package main

import (
	"encoding/json"
	"net/http"
)

// Widget is the control DTO — nothing in its name collides.
type Widget struct {
	ID string `json:"id"`
}

// StubQuote is a real DTO: an indicative ("stub") price quote shown before the
// pricing engine runs. Its name collides with the "stub" substring.
type StubQuote struct {
	Price int `json:"price"`
}

// MockDataService serves canned data to the sandbox tenant — a shipped feature,
// not a test double. Its receiver name collides with "mock".
type MockDataService struct{}

// Serve is a handler method whose receiver name collides: the method was
// dropped from the type's method table, and the type from the Types map.
// This route's own schema survived that (the encode call is attributed to
// Serve, which does not collide), so it stands as coverage for the method
// filter rather than as a reproduction of it.
func (s *MockDataService) Serve(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Widget{})
}

// healthHandler is the control: a plainly-named handler encoding Widget.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Widget{})
}

// fakeDoorHandler backs a fake-door A/B test. Its function name collides, which
// also made it a colliding *caller*, so the encode call below was dropped too.
func fakeDoorHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Widget{})
}

// quoteHandler is plainly named but returns a colliding *type*. The type
// filter dropped StubQuote from the Types table; the schema happened to
// survive because it is rebuilt from the composite literal at the use site.
// Kept as coverage for the declaration filter all the same.
func quoteHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(StubQuote{})
}

// writeStubQuote is a colliding *callee*: a plainly-named handler calling it
// used to lose the edge, and with it the response body.
func writeStubQuote(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(StubQuote{})
}

// indicativeHandler is plainly named and reaches its body through the colliding
// helper above.
func indicativeHandler(w http.ResponseWriter, r *http.Request) {
	writeStubQuote(w)
}

func main() {
	svc := &MockDataService{}

	http.HandleFunc("GET /health", healthHandler)
	http.HandleFunc("GET /fake-door", fakeDoorHandler)
	http.HandleFunc("GET /quote", quoteHandler)
	http.HandleFunc("GET /indicative", indicativeHandler)
	http.HandleFunc("GET /sandbox/data", svc.Serve)

	http.ListenAndServe(":8080", nil)
}
