// Package main is the 2x2 of how a builder's path reaches its registration
// (issue #506), because two independent gaps compose here and a fixture that
// mixes them cannot tell them apart.
//
// The builder holds the path on its receiver and registers from it two ways:
//
//	c.r.mux.HandleFunc("GET "+c.pattern, h)   // a literal concatenated on
//	c.r.mux.HandleFunc(c.pattern, h)          // the field IS the whole argument
//
// and a caller reaches the builder two ways: chained straight off the
// constructor, or assigned to a variable first.
//
//	                      concatenated        bare selector
//	chained               /direct-concat      /direct-bare
//	assigned to a var     /var-concat         /var-bare
//
// Only /direct-concat used to resolve. Worse, only /var-concat was REPORTED —
// the two bare shapes produced no path, no placeholder and no diagnostic, so
// the document simply omitted them and read as finished.
package main

import (
	"encoding/json"
	"net/http"
)

// Item is what these routes return.
type Item struct {
	ID string `json:"id"`
}

type Router struct{ mux *http.ServeMux }

// Combo carries the path so several verbs can be registered from it.
type Combo struct {
	r       *Router
	pattern string
}

func (r *Router) Combo(pattern string) *Combo { return &Combo{r, pattern} }

// Get concatenates a literal onto the receiver field.
func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("GET "+c.pattern, h)
	return c
}

// Handle passes the receiver field as the WHOLE path argument.
func (c *Combo) Handle(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc(c.pattern, h)
	return c
}

func main() {
	r := &Router{mux: http.NewServeMux()}

	// Chained straight off the constructor.
	r.Combo("/direct-concat").Get(listDirectConcat)
	r.Combo("/direct-bare").Handle(listDirectBare)

	// Assigned to a variable first, which is how a builder is written whenever
	// more than one line's worth of registration hangs off it.
	vc := r.Combo("/var-concat")
	vc.Get(listVarConcat)

	vb := r.Combo("/var-bare")
	vb.Handle(listVarBare)

	// Control: a literal registration, which never went through the builder.
	r.mux.HandleFunc("GET /plain", listPlain)

	_ = http.ListenAndServe(":8080", r.mux)
}

func listDirectConcat(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}
func listDirectBare(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }
func listVarConcat(w http.ResponseWriter, r *http.Request)  { _ = json.NewEncoder(w).Encode([]Item{}) }
func listVarBare(w http.ResponseWriter, r *http.Request)    { _ = json.NewEncoder(w).Encode([]Item{}) }
func listPlain(w http.ResponseWriter, r *http.Request)      { _ = json.NewEncoder(w).Encode([]Item{}) }
