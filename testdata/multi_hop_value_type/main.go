// Fixture: a concrete value forwarded through TWO OR MORE helper hops typed
// `any` must keep its concrete type in the response/request schema (issue #180).
// The response value-type resolution previously walked only a single parameter
// hop, so `encOuter(w, User{})` (which forwards to `encInner`) erased User to a
// generic object. Multi-hop parameter tracing recovers the concrete type at any
// depth. The request side (which already traced multi-hop) is included as a
// symmetric guard.
package main

import (
	"encoding/json"
	"io"
	"net/http"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Response helper chain: encThree -> encOuter -> encInner -> Encode(v).
func encInner(dst io.Writer, v any) { _ = json.NewEncoder(dst).Encode(v) }
func encOuter(dst io.Writer, v any) { encInner(dst, v) }
func encThree(dst io.Writer, v any) { encOuter(dst, v) }

// getOneHop resolves through a single helper hop (the control).
func getOneHop(w http.ResponseWriter, r *http.Request) { encInner(w, User{ID: 1}) }

// getTwoHops forwards User through two `any` parameter boundaries.
func getTwoHops(w http.ResponseWriter, r *http.Request) { encOuter(w, User{ID: 2}) }

// getThreeHops forwards User through three `any` parameter boundaries.
func getThreeHops(w http.ResponseWriter, r *http.Request) { encThree(w, User{ID: 3}) }

// Request helper chain: decOuter -> decInner -> Decode(v).
func decInner(src io.Reader, v any) error { return json.NewDecoder(src).Decode(v) }
func decOuter(src io.Reader, v any) error { return decInner(src, v) }

// createTwoHops decodes the body through two `any` parameter boundaries.
func createTwoHops(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = decOuter(r.Body, &req)
	_ = req
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /one-hop", getOneHop)
	mux.HandleFunc("GET /two-hops", getTwoHops)
	mux.HandleFunc("GET /three-hops", getThreeHops)
	mux.HandleFunc("POST /create-two-hops", createTwoHops)
	_ = http.ListenAndServe(":8080", mux)
}
