// Fixture: status through a constructor field, where the handler is a closure
// returned by a METHOD (the handler-factory shape common in clean-architecture
// Go):
//
//	func (h *handler) Login() http.HandlerFunc {
//	    return func(w, r) {
//	        e := NewAPIError("missing token", http.StatusUnauthorized)
//	        RespondWithError(w, e)          // w.WriteHeader(err.Code)
//	    }
//	}
//
// The `e := NewAPIError(...)` assignment lives inside the closure, but the
// metadata records it on the *enclosing method's* AssignmentMap (ast.Inspect
// descends into the literal). The status resolver used to look the assignment
// up only via the FuncLit caller's function record — which doesn't exist for a
// method — so the 401 collapsed to `default`. It must resolve through the
// edge's ParentFunction, including methods (Type.Methods, not file.Functions).
// The plain-function variant already worked (testdata/status_via_constructor);
// this fixture pins the method-closure variant.
package main

import (
	"encoding/json"
	"net/http"
)

// APIError is the error body every failure path encodes.
type APIError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewAPIError is the constructor carrying the status into a struct field.
func NewAPIError(message string, code int) *APIError {
	return &APIError{Message: message, Code: code}
}

// RespondWithError writes the error with its embedded status.
func RespondWithError(w http.ResponseWriter, err *APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	_ = json.NewEncoder(w).Encode(err)
}

// Profile is the success body.
type Profile struct {
	Email string `json:"email"`
}

type handler struct{}

// Login returns the actual handler as a closure (handler-factory).
func (h *handler) Login() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") == "" {
			e := NewAPIError("missing token", http.StatusUnauthorized)
			RespondWithError(w, e)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Profile{Email: "a@b.c"})
	}
}

func main() {
	h := &handler{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /profile", h.Login())
	_ = http.ListenAndServe(":8080", mux)
}
