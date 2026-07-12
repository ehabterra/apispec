// Package main exercises control-flow method dispatch: a single net/http
// handler registered without a verb that branches on r.Method (via switch or
// if) must split into one OpenAPI operation per HTTP method, with each branch's
// request/response attributed to its own method.
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

// usersHandler dispatches on r.Method with a switch: GET lists users, POST
// creates one (with a request body and a 201), and default rejects.
func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode([]User{})
	case http.MethodPost:
		var req CreateUserRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(User{})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// itemHandler dispatches on r.Method with an if/else-if chain: GET returns a
// user, DELETE returns 204 No Content.
func itemHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(User{})
	} else if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
	}
}

// pingHandler uses a single case that lists multiple methods; both GET and HEAD
// map to the same 200 branch.
func pingHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		w.WriteHeader(http.StatusOK)
	}
}

func main() {
	http.HandleFunc("/users", usersHandler)
	http.HandleFunc("/item", itemHandler)
	http.HandleFunc("/ping", pingHandler)
	http.ListenAndServe(":8080", nil)
}
