package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(r.Context())
		next.ServeHTTP(w, r)
	})
}

func routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(requestContext)
			r.Get("/tenant", tenantHandler)
			mountCapabilities(r)
			r.Group(func(r chi.Router) {
				r.Use(requestContext)
				mountUsers(r)
			})
		})
	})
	return r
}

func mountCapabilities(r chi.Router) {
	r.Get("/capabilities", capabilitiesHandler)
}

func mountUsers(r chi.Router) {
	r.Get("/users", usersHandler)
}

func tenantHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func capabilitiesHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func usersHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	http.ListenAndServe(":8080", routes())
}
