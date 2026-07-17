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

// Animal is an interface returned by makeAnimal; Dog is the concrete value. When
// makeAnimal() is forwarded through the helper chain, resolving the concrete Dog
// requires reading the scope where makeAnimal is CALLED (the handler), not the
// deepest helper's scope — the multi-hop trace must carry the resolved node.
type Animal interface{ Sound() string }

type Dog struct {
	Name string `json:"name"`
}

func (Dog) Sound() string { return "woof" }

func makeAnimal() Animal { return Dog{} }

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

// getInterfaceReturn forwards an interface-returning CALL through two hops; the
// concrete Dog resolves only if the trace carries the handler's scope.
func getInterfaceReturn(w http.ResponseWriter, r *http.Request) { encOuter(w, makeAnimal()) }

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
	mux.HandleFunc("GET /interface-return", getInterfaceReturn)
	mux.HandleFunc("POST /create-two-hops", createTwoHops)
	_ = http.ListenAndServe(":8080", mux)
}
