// Fixture for issue #168: a handler's Go doc comment becomes the operation
// summary (first line) + description (rest). Covers every handler shape —
// package-level func, pointer-receiver method, value-receiver method — plus the
// shapes that must stay empty (undocumented method, func literal).
package main

import (
	"encoding/json"
	"net/http"
)

type Account struct {
	ID string `json:"id"`
}

type Handler struct{}

// CreateAccount registers a new account.
// It validates the payload and returns the created account.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var a Account
	_ = json.NewDecoder(r.Body).Decode(&a)
	_ = json.NewEncoder(w).Encode(a)
}

// DeleteAccount removes an account.
func (h Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PatchAccount(w http.ResponseWriter, r *http.Request) {
	// Deliberately undocumented: the operation keeps an empty summary rather
	// than sourcing from a non-doc comment.
	w.WriteHeader(http.StatusOK)
}

// listAccounts returns every account.
// The remaining lines become the operation description.
func listAccounts(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Account{})
}

// Deps carries the handler as a field, mirroring dependency-injected routers:
// the receiver then renders as a field path (Deps.Accounts), not a type name.
type Deps struct {
	Accounts *Handler
}

func main() {
	h := &Handler{}
	deps := Deps{Accounts: h}
	mux := http.NewServeMux()
	mux.HandleFunc("HEAD /accounts", deps.Accounts.CreateAccount)
	mux.HandleFunc("POST /accounts", h.CreateAccount)
	mux.HandleFunc("DELETE /accounts", h.DeleteAccount)
	mux.HandleFunc("PATCH /accounts", h.PatchAccount)
	mux.HandleFunc("GET /accounts", listAccounts)
	// A func literal has no doc comment to source from.
	mux.HandleFunc("PUT /accounts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = http.ListenAndServe(":8080", mux)
}
