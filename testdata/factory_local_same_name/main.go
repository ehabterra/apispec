// Fixture: a router built by a factory and mounted by the caller, where the
// factory's local is named like the caller's variable (issue #628):
//
//	/a     same := Same()      factory local `same`
//	/b     other := Other()    factory local `m` (control)
//	/c     Mount("/c", Same2()) inline, factory local `same2`
//	/x     rt := sub.Routes()  another package, factory local `rt`
//	/h     same := Same3()     the same shape in a helper, not in main
//
// Every route belongs under its mount prefix; none at the root.
package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/factory_local_same_name/sub"
	"github.com/go-chi/chi/v5"
)

func h(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func Same() *chi.Mux  { same := chi.NewRouter(); same.Get("/same", h); return same }
func Other() *chi.Mux { m := chi.NewRouter(); m.Get("/other", h); return m }
func Same2() *chi.Mux { same2 := chi.NewRouter(); same2.Get("/inline", h); return same2 }
func Same3() *chi.Mux { same := chi.NewRouter(); same.Get("/helper", h); return same }

func mountHelper(r chi.Router) {
	same := Same3()
	r.Mount("/h", same)
}

func main() {
	r := chi.NewRouter()

	same := Same()
	r.Mount("/a", same)

	other := Other()
	r.Mount("/b", other)

	r.Mount("/c", Same2())

	rt := sub.Routes()
	r.Mount("/x", rt)

	mountHelper(r)

	_ = http.ListenAndServe(":8080", r)
}
