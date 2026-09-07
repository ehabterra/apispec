// Package main exercises a path held in a field of the RECEIVER.
//
// A combo builder — gitea's `web.Combo` is one — takes the path once and
// registers several verbs from it:
//
//	r.Combo("/items").Get(list).Post(create)
//
// Inside Get/Post the path is `c.pattern`, a field read off the receiver. That
// is not the shape the ladder's struct-field rung answers (a field off a struct
// the CALLER passed), so nothing resolved — and the argument was rendered,
// which put the Go symbol `recvfield.Combo.pattern` in the path as though it
// were a literal segment. 60 paths on a real project looked like that
// (issue #461).
package main

import (
	"encoding/json"
	"net/http"
)

// Item is what these routes return.
type Item struct {
	ID string `json:"id"`
}

// Config holds a path in a field, which is how a house router or a settings
// struct usually carries one.
type Config struct{ Path string }

// settings is a package-level value whose field is used as a path.
var settings = Config{Path: "/from-field"}

// Router is a thin house wrapper over ServeMux.
type Router struct{ mux *http.ServeMux }

// Combo carries the path so several verbs can be registered from it.
type Combo struct {
	r       *Router
	pattern string
}

// Combo takes the path once. Every verb below reads it back off the receiver.
func (r *Router) Combo(pattern string) *Combo { return &Combo{r, pattern} }

func (c *Combo) Get(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("GET "+c.pattern, h)
	return c
}

func (c *Combo) Post(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc("POST "+c.pattern, h)
	return c
}

// Handle passes the receiver field as the WHOLE path argument, with no literal
// concatenated onto it. That is the shape that was rendered: a bare selector
// reached resolvePathArg's default branch, which rendered the argument.
func (c *Combo) Handle(h http.HandlerFunc) *Combo {
	c.r.mux.HandleFunc(c.pattern, h)
	return c
}

func listItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func createItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{})
}

// plainList is registered directly, as the control: its path is a literal and
// must be unaffected.
func plainList(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func main() {
	r := &Router{mux: http.NewServeMux()}

	r.Combo("/items").Get(listItems).Post(createItem)

	// The bare-selector shape, assigned first (a value created inside an
	// argument list has no assignment to key on — golden rule #11).
	things := r.Combo("/things")
	things.Handle(listItems)

	// A bare SELECTOR as the whole path argument. This is the shape that was
	// rendered: `resolvePathArg` had no case for it, so the argument was
	// stringified and a Go symbol became the path.
	r.mux.HandleFunc(settings.Path, plainList)

	r.mux.HandleFunc("GET /plain", plainList)

	_ = http.ListenAndServe(":8080", r.mux)
}
