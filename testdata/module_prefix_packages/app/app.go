package app

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Session is returned by the app package.
type Session struct {
	Token string `json:"token"`
	User  string `json:"user"`
}

// Register mounts the app routes.
func Register(r chi.Router) {
	r.Get("/sessions/{token}", getSession)
}

func getSession(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Session{Token: chi.URLParam(r, "token")})
}
