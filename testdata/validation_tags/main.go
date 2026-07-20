package main

import (
	"encoding/json"
	"net/http"
)

// CreateAccountRequest exercises validator-tag fidelity (issues #165, #166, #167).
type CreateAccountRequest struct {
	// String min/max constrain LENGTH → minLength / maxLength (#167).
	Name string `json:"name" validate:"required,min=3,max=50"`
	// Numeric min/max constrain VALUE → minimum / maximum.
	Age int `json:"age" validate:"min=18,max=120"`
	// Slice min/max constrain ITEM COUNT → minItems / maxItems; the post-`dive`
	// rules constrain each element → items.minimum / items.maximum (#165).
	Scores []int `json:"scores" validate:"required,min=1,max=10,dive,min=5,max=100"`
}

func createAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", createAccount)
	_ = http.ListenAndServe(":8080", mux)
}
