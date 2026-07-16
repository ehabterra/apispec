// Fixture: request body decoded through a helper that assigns the decoder to a
// local variable before calling Decode — the common
//
//	dec := json.NewDecoder(r.Body)
//	dec.DisallowUnknownFields()
//	dec.Decode(dst)
//
// shape (e.g. a project's decodeJSON(w, r, &req) wrapper). The intermediate
// `dec` variable re-homes dec.Decode under the json.NewDecoder producer in the
// tracker tree, so a single-hop parent lookup could not reach the wrapper's
// `dst` parameter and the body collapsed to a generic object. The concrete
// type must resolve through the wrapper to a $ref. An inline decoder (no
// variable) already worked and is kept here as the control.
package main

import (
	"encoding/json"
	"net/http"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UpdateUserRequest struct {
	Name string `json:"name"`
}

// decodeJSON assigns the decoder to a variable (the regressed shape).
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", createUser)
	mux.HandleFunc("PUT /users/{id}", updateUser)
	_ = http.ListenAndServe(":8080", mux)
}
