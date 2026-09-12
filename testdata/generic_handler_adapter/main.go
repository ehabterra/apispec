// Package main exercises a generic handler adapter — the shape several Go HTTP
// toolkits ship and many projects hand-roll once and reuse everywhere. The
// response type is a TYPE PARAMETER, resolved from the instantiation at the
// registration (issue #367).
package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ListResponse struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
}

type Health struct {
	OK bool `json:"ok"`
}

// HandleJSON adapts a typed function into an http.HandlerFunc. `in` is a
// declared variable of type Req, which already resolves; `out` is the RESULT of
// calling a func-typed parameter, which is the half that did not.
func HandleJSON[Req any, Res any](fn func(context.Context, Req) (Res, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in Req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := fn(r.Context(), in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}

// HandleJSONResponse is the response-only variant: one type parameter, no
// request body.
func HandleJSONResponse[Res any](fn func(context.Context) (Res, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := fn(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}

func createUser(_ context.Context, in CreateUserRequest) (User, error) {
	return User{ID: 1, Name: in.Name}, nil
}

func listUsers(_ context.Context) (ListResponse, error) {
	return ListResponse{}, nil
}

func health(_ context.Context) (Health, error) {
	return Health{OK: true}, nil
}

func main() {
	mux := http.NewServeMux()

	// Two instantiations of the same adapter: each operation must document ITS
	// OWN response type, not a shared component named after the parameter.
	mux.Handle("POST /users", HandleJSON(createUser))
	mux.Handle("GET /users", HandleJSONResponse(listUsers))
	mux.Handle("GET /health", HandleJSONResponse(health))

	_ = http.ListenAndServe(":8080", mux)
}
