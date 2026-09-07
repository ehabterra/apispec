// Package main is a house router whose registration is CHAINED through a
// returned object: `Combo("/items").Get(h).Post(h)` holds the pattern on the
// returned value, so by the time the framework call runs, the path is a field
// read rather than a literal.
//
// Wrapper derivation declines this shape — `--verbose` reports it as
// "incomplete, not applied" — and the framework call inside the wrapper was
// then matched on its own and documented at `/{pattern}` (issue #428), then
// reported and left out.
//
// It now RESOLVES: the chain leads back to the call that built the receiver,
// whose returned literal stores the constructor's parameter, and the argument
// bound to that parameter is recorded — so `/items` is derived from facts
// rather than guessed (issue #461). The keyed literal here (`&Combo{r: r,
// pattern: pattern}`) is the counterpart to receiver_field_path's positional
// one; both spellings must resolve.
//
// The ordinary method on the SAME router must still resolve, which is what
// separates "this wiring style is not supported" from "this router is not".
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Router is the house router.
type Router struct {
	mux *http.ServeMux
}

// Combo starts a chained registration: the pattern travels on the returned
// object rather than into the framework call.
func (r *Router) Combo(pattern string) *Combo {
	return &Combo{r: r, pattern: pattern}
}

// Get registers a plain route, forwarding its own parameters — the shape
// wrapper derivation does resolve.
func (r *Router) Get(pattern string, h http.HandlerFunc) {
	r.mux.HandleFunc("GET "+pattern, h)
}

// Combo carries the pattern between chained calls.
type Combo struct {
	r       *Router
	pattern string
}

// Get registers the chained GET.
func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("GET "+c.pattern, h)
	return c
}

// Post registers the chained POST.
func (c *Combo) Post(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("POST "+c.pattern, h)
	return c
}

func listItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func createItem(w http.ResponseWriter, r *http.Request) {
	var in Item
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	rt := &Router{mux: http.NewServeMux()}

	// Dropped: the path is read off the chained object.
	rt.Combo("/items").Get(listItems).Post(createItem)

	// Kept: the pattern is forwarded from the wrapper's own parameter.
	rt.Get("/health", health)

	http.ListenAndServe(":8080", rt.mux)
}
