// Package main registers through a builder declared in ANOTHER package.
//
// This is the ordinary shape on a real project and the one a single-package
// fixture cannot produce. Two facts only exist here:
//
//   - the constructor is reached by a SELECTOR call (`r.NewCombo`) whose
//     returned literal states its type as a selector too (`&web.Combo{…}`),
//     not as a bare ident. Anything reading a name off the ident alone gets
//     "" for both (golden rule #10);
//   - the receiver field belongs to a type in a package that is not the
//     registration's, so the package half of the identity check is doing work
//     rather than comparing a package with itself.
package main

import (
	"encoding/json"
	"net/http"

	"buildercrosspackage/web"
)

// Item is what these routes return.
type Item struct {
	ID string `json:"id"`
}

func main() {
	r := &web.Router{Mux: http.NewServeMux()}

	// Chained straight off the constructor.
	r.NewCombo("/direct").Get(listDirect)

	// Assigned to a variable first.
	v := r.NewCombo("/assigned")
	v.Get(listAssigned)
	v.Post(createAssigned)

	// Control: a literal registration that never went through the builder.
	r.Mux.HandleFunc("GET /plain", listPlain)

	_ = http.ListenAndServe(":8080", r.Mux)
}

func listDirect(w http.ResponseWriter, r *http.Request)   { _ = json.NewEncoder(w).Encode([]Item{}) }
func listAssigned(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }
func listPlain(w http.ResponseWriter, r *http.Request)    { _ = json.NewEncoder(w).Encode([]Item{}) }

func createAssigned(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{})
}
