// Package main exercises control-flow method dispatch written in a CLOSURE
// rather than in a named handler (issue #382). A function literal has no
// Function record in the metadata, so its `switch r.Method` used to be folded
// into the enclosing declaration's — invisible from the route, which then fell
// to the POST default with every branch's responses merged onto it.
//
// The shapes here are deliberately side by side: two closures registered from
// one function (so a per-FILE scope would mix their arms), a closure whose arms
// call methods (so the arm has to be found on the call chain, not at the
// response statement), a named factory returning the same closure, a closure
// inside a method (which is not in Functions at all), and an explicit-verb
// registration that must NOT split.
package main

import (
	"encoding/json"
	"net/http"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

type UpdateUserRequest struct {
	Name string `json:"name"`
}

type handlers struct{}

// Get writes the 200 body one call away from the arm that reaches it.
func (h *handlers) Get(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

// Create reads the request body and answers 201, again one call from the arm.
func (h *handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.WriteHeader(http.StatusCreated)
}

// namedMethods is the factory shape: the dispatch is in the returned closure,
// while the route's handler resolves to the factory.
func namedMethods(h *handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.Get(w, r)
		case http.MethodPost:
			h.Create(w, r)
		}
	}
}

type server struct {
	h *handlers
}

// routes registers from a METHOD, whose body holds no Function record — the
// closure's dispatch has to be recorded from a whole-file walk to be found.
func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("/from-method", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.h.Get(w, r)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

func main() {
	h := &handlers{}

	// (a) inline arm bodies: the response statements are in the arms.
	http.HandleFunc("/inline", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(User{})
		case http.MethodPost:
			var req CreateUserRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// (b) a second closure in the SAME function: its arms must not leak into
	// the one above, which a file-scoped attribution would let them do.
	http.HandleFunc("/inline-second", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var req UpdateUserRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(User{})
		} else if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// (c) arms that call methods: the response is written a frame deeper, so
	// the arm is only visible on the call chain.
	http.HandleFunc("/via-methods", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.Get(w, r)
		case http.MethodPost:
			h.Create(w, r)
		}
	})

	// (d) the factory shape.
	http.HandleFunc("/from-factory", namedMethods(h))

	// (e) an explicit verb: the router only sends GET here, so the closure's
	// POST arm is dead for this route and the route must stay a single GET.
	http.HandleFunc("GET /explicit", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(User{})
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		}
	})

	mux := http.NewServeMux()
	(&server{h: h}).routes(mux)

	http.ListenAndServe(":8080", nil)
}
