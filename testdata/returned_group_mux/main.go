// Fixture: the gorilla/mux form of a subrouter created inside a helper and
// returned to the caller (issue #633):
//
//	/api/items        return r.PathPrefix("/api").Subrouter()
//	/admin/users      s := r.PathPrefix("/admin").Subrouter(); return s
package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func items(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func users(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func API(r *mux.Router) *mux.Router { return r.PathPrefix("/api").Subrouter() }

func Admin(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/admin").Subrouter()
	return s
}

func main() {
	r := mux.NewRouter()

	api := API(r)
	api.HandleFunc("/items", items).Methods(http.MethodGet)

	admin := Admin(r)
	admin.HandleFunc("/users", users).Methods(http.MethodGet)

	_ = http.ListenAndServe(":8080", r)
}
