// Fixture: write-destination gating (issue #170). A value encoded to some other
// io.Writer — a bytes.Buffer, a hash — must NOT be attributed as the operation
// response. Only a value whose encoder destination provably traces to the HTTP
// response writer becomes the response. This mirrors the request-side source
// gating (issue #153): request gating traces a decoder's source to r.Body;
// response gating traces an encoder's destination to w.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
)

// User is the real response body.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Secret is encoded only to a local buffer — it must never surface as a response.
type Secret struct {
	Token string `json:"token"`
}

// Audit is encoded to a hash writer — also never a response.
type Audit struct {
	Event string `json:"event"`
}

// getUser encodes User straight to the response writer — the real response.
func getUser(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{ID: "1", Name: "a"})
}

// getUserViaHelper threads w through a wrapper before encoding — still a
// response (the destination traces to w across the parameter boundary).
func getUserViaHelper(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, User{ID: "2", Name: "b"})
}

func writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

// getUserViaIOWriter threads w through a helper whose parameter is typed
// io.Writer — the destination can't be PROVEN to be the response writer, but it
// could be (a writer satisfies io.Writer), so the response must be kept. This
// is the ubiquitous writeJSON(w io.Writer, v) shape; dropping it was the
// regression the conservative gate avoids.
func getUserViaIOWriter(w http.ResponseWriter, r *http.Request) {
	encodeTo(w, User{ID: "3", Name: "c"})
}

func encodeTo(dst io.Writer, v any) {
	_ = json.NewEncoder(dst).Encode(v)
}

// leakBuffer encodes Secret to a bytes.Buffer, not to w — must be ignored. The
// real response is the plain w.Write below.
func leakBuffer(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(Secret{Token: "shh"})
	_, _ = w.Write([]byte("ok"))
}

// leakHash encodes Audit into a hash — must be ignored.
func leakHash(w http.ResponseWriter, r *http.Request) {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(Audit{Event: "login"})
	_, _ = w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", getUser)
	mux.HandleFunc("GET /user-helper", getUserViaHelper)
	mux.HandleFunc("GET /user-iowriter", getUserViaIOWriter)
	mux.HandleFunc("POST /leak-buffer", leakBuffer)
	mux.HandleFunc("POST /leak-hash", leakHash)
	_ = http.ListenAndServe(":8080", mux)
}
