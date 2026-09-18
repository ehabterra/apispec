// Package main covers a builder that registers every route it makes from ONE
// call site (issue #465).
//
//	func (c *Combo) Get(h http.HandlerFunc) *Combo {
//		c.r.mux.HandleFunc("GET "+c.pattern, h)   // ← the only registration
//		return c
//	}
//
// Two chains therefore reach the same node with the same key and the same
// (empty) mount path. Both gates that dedupe the route walk keyed on exactly
// that, so the second chain was dropped twice over: the traversal skipped it as
// already-visited, and the extractor skipped it as already-extracted. The
// document stated the first chain's path confidently and said nothing at all
// about the rest — no placeholder, no warning, nothing to notice.
//
// The shape is not exotic. It is how gitea's web.Combo works, and recovering it
// adds 18 endpoints there.
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

// Get and Post each register from a single call site, shared by every chain.
func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("GET "+c.pattern, h)
	return c
}

func (c *Combo) Post(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("POST "+c.pattern, h)
	return c
}

func main() {
	r := &Router{mux: http.NewServeMux()}

	// THREE chains through the same Get call site. One used to survive.
	r.Combo("/alpha").Get(listAlpha)
	r.Combo("/beta").Get(listBeta)
	r.Combo("/gamma").Get(listGamma)

	// A chain registering two verbs from two call sites, each also shared with
	// the chains above — so the same node is reached by four different chains.
	r.Combo("/items").Get(listItems).Post(createItem)

	// Control: a literal registration, which never went through a chain.
	r.mux.HandleFunc("GET /plain", listPlain)

	_ = http.ListenAndServe(":8080", r.mux)
}

func listAlpha(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }
func listBeta(w http.ResponseWriter, r *http.Request)  { _ = json.NewEncoder(w).Encode([]Item{}) }
func listGamma(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }
func listItems(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }

func createItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{})
}

func listPlain(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Item{}) }
