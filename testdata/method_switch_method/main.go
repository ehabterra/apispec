// Package main exercises control-flow method dispatch written in a METHOD
// (issue #427). Methods are not in File.Functions — processFunctions skips any
// declaration with a receiver — so a handler like `h.Users` could not be
// resolved to its dispatch, and the route fell to the POST default with every
// arm's responses merged onto it.
//
// The shapes here are the ones that resolve differently: a pointer receiver, a
// value receiver (whose identity renders with the metadata type separator), a
// method in another package, a method whose arms DELEGATE to other methods
// (attributed through the call chain), and an explicit-verb registration that
// must not split.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/ehabterra/apispec/testdata/method_switch_method/api"
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

// PtrHandler serves with a pointer receiver.
type PtrHandler struct{}

// Users lists or creates users.
func (h *PtrHandler) Users(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode([]User{})
	case http.MethodPost:
		var req CreateUserRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// get and update are the arms' delegates: their bodies are written outside the
// dispatching method, so only the call in the arm says which verb reaches them.
func (h *PtrHandler) get(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

func (h *PtrHandler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = json.NewEncoder(w).Encode(User{})
}

// User dispatches to other methods rather than answering inline.
func (h *PtrHandler) User(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPut:
		h.update(w, r)
	}
}

// ValHandler serves with a value receiver.
type ValHandler struct{}

// Things reads or replaces a thing.
func (h ValHandler) Things(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(User{})
	} else if r.Method == http.MethodPatch {
		w.WriteHeader(http.StatusAccepted)
	}
}

// Explicit is registered with a concrete verb, so only its GET arm is live.
func (h *PtrHandler) Explicit(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(User{})
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
	}
}

func main() {
	p := &PtrHandler{}
	v := ValHandler{}
	a := &api.API{}

	http.HandleFunc("/users", p.Users)
	http.HandleFunc("/user", p.User)
	http.HandleFunc("/things", v.Things)
	http.HandleFunc("/items", a.Items)
	http.HandleFunc("GET /explicit", p.Explicit)

	http.ListenAndServe(":8080", nil)
}
