// Fixture: write-destination gating by PROVENANCE (issue #170). An encode is the
// operation response only when its destination traces back — through parameters,
// assignments, and struct construction — to the handler's response-writer
// parameter `w`. A value encoded to any other sink (a bytes.Buffer, a hash,
// io.Discard) is not the response. The response writer's identity is already in
// the code (the `w` parameter); classification is value-flow tracking, not type
// guessing. Mirror of the request-body source gating (#153, which traces a
// decoder's source to r.Body).
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
)

// User is the real response body; Secret must never reach the response.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Secret struct {
	Token string `json:"token"`
}

// loggingWriter is a custom ResponseWriter wrapper embedding the real writer.
type loggingWriter struct {
	http.ResponseWriter
}

// encodeTo is a generic helper whose destination is an io.Writer parameter.
func encodeTo(dst io.Writer, v any) { _ = json.NewEncoder(dst).Encode(v) }

func makeBuffer() *bytes.Buffer { return &bytes.Buffer{} }

// newLoggingWriter is a CONSTRUCTOR-FUNCTION wrapper around w (the common
// middleware pattern). The writer flows into the call, so the result wraps it.
func newLoggingWriter(w http.ResponseWriter) *loggingWriter { return &loggingWriter{w} }

// --- KEEP: the destination traces to the response writer w ---

// getDirect writes straight to w.
func getDirect(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{ID: "1"})
}

// getViaHelper threads w through an io.Writer helper parameter.
func getViaHelper(w http.ResponseWriter, r *http.Request) {
	encodeTo(w, User{ID: "2"})
}

// getViaAssign encodes through a local aliased to w.
func getViaAssign(w http.ResponseWriter, r *http.Request) {
	dst := w
	_ = json.NewEncoder(dst).Encode(User{ID: "3"})
}

// getViaWrapper encodes to a wrapper struct constructed around w.
func getViaWrapper(w http.ResponseWriter, r *http.Request) {
	lw := &loggingWriter{w}
	_ = json.NewEncoder(lw).Encode(User{ID: "4"})
}

// getViaCtorWrapper encodes to a wrapper built by a CONSTRUCTOR FUNCTION — the
// writer flows through the call argument, so the response must be kept (the
// regression that dropped these was surfacing as spurious `default` statuses).
func getViaCtorWrapper(w http.ResponseWriter, r *http.Request) {
	lw := newLoggingWriter(w)
	_ = json.NewEncoder(lw).Encode(User{ID: "5"})
}

// --- DROP: the destination is a sink unrelated to w ---

// leakBuffer encodes to a bytes.Buffer.
func leakBuffer(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	encodeTo(&buf, Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

// leakHash encodes into a hash.
func leakHash(w http.ResponseWriter, r *http.Request) {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

// leakDiscard encodes to io.Discard.
func leakDiscard(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(io.Discard).Encode(Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

// leakConstructed encodes to a buffer returned by a constructor.
func leakConstructed(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(makeBuffer()).Encode(Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

// leakRecorder encodes to a locally-built httptest recorder — writer-typed but
// NOT the handler's w, so it has no response provenance.
func leakRecorder(w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	_ = json.NewEncoder(rec).Encode(Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /direct", getDirect)
	mux.HandleFunc("GET /helper", getViaHelper)
	mux.HandleFunc("GET /assign", getViaAssign)
	mux.HandleFunc("GET /wrapper", getViaWrapper)
	mux.HandleFunc("GET /ctor-wrapper", getViaCtorWrapper)
	mux.HandleFunc("POST /leak-buffer", leakBuffer)
	mux.HandleFunc("POST /leak-hash", leakHash)
	mux.HandleFunc("POST /leak-discard", leakDiscard)
	mux.HandleFunc("POST /leak-constructed", leakConstructed)
	mux.HandleFunc("POST /leak-recorder", leakRecorder)
	_ = http.ListenAndServe(":8080", mux)
}
