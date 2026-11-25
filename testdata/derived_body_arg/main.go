// Package main holds responders whose body argument is BUILT from a parameter
// rather than being one (issue #659).
//
// `http.Error(w, "bad "+p, 400)` with `p := req.URL.Path` mentions `req`, but
// the body is a string the method made, not the *http.Request it was made
// from. Mapping the body role back to `req` documented the request type as the
// response body of every caller.
package main

import (
	"encoding/json"
	"net/http"
)

// R is a router-like receiver carrying the project's own response helpers.
type R struct{}

// normalize is the issue's shape: the message is derived from a selector into
// the request, through a local.
func (rt *R) normalize(w http.ResponseWriter, req *http.Request) {
	p := req.URL.Path
	http.Error(w, "bad "+p, http.StatusBadRequest)
}

// rejectPath is the function-shaped variant: no receiver, and the selector is
// written straight into the message.
func rejectPath(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "bad path "+req.URL.Path, http.StatusBadRequest)
}

// send forwards the caller's value unchanged: the caller's type IS the body.
func (rt *R) send(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

// sendRef forwards it through a plain local and its address — still the
// caller's value.
func (rt *R) sendRef(w http.ResponseWriter, v any) {
	out := v
	_ = json.NewEncoder(w).Encode(&out)
}

// fail forwards both the message and the status unchanged.
func (rt *R) fail(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, code)
}

// failCode derives its message FROM the status: the body is the status text,
// a string, never the integer it was computed from.
func (rt *R) failCode(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

// bind decodes into the caller's destination: the caller's type IS the
// request body.
func (rt *R) bind(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// bindEnvelope decodes into an envelope it builds around the destination: the
// request body is the Envelope, not the bare value the caller hands in.
func (rt *R) bindEnvelope(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(&Envelope{Data: dst})
}

// Envelope wraps every payload bindEnvelope reads.
type Envelope struct {
	Data any `json:"data"`
}

// Order is the payload handed to bindEnvelope.
type Order struct {
	Qty int `json:"qty"`
}

// Item is the body send documents.
type Item struct {
	ID int `json:"id"`
}

// Widget is the body sendRef documents.
type Widget struct {
	Name string `json:"name"`
}

func normalized(w http.ResponseWriter, r *http.Request) {
	rt := &R{}
	if r.URL.Query().Get("strict") != "" {
		rt.normalize(w, r)
		return
	}
	rt.send(w, Item{})
}

func rejected(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("strict") != "" {
		rejectPath(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(Item{})
}

func widgets(w http.ResponseWriter, r *http.Request) {
	rt := &R{}
	if r.URL.Query().Get("missing") != "" {
		rt.fail(w, "no such widget", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("taken") != "" {
		rt.failCode(w, http.StatusConflict)
		return
	}
	rt.sendRef(w, Widget{})
}

func createWidget(w http.ResponseWriter, r *http.Request) {
	var in Widget
	if err := (&R{}).bind(r, &in); err != nil {
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func createOrder(w http.ResponseWriter, r *http.Request) {
	var in Order
	if err := (&R{}).bindEnvelope(r, &in); err != nil {
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /normalized", normalized)
	mux.HandleFunc("GET /rejected", rejected)
	mux.HandleFunc("GET /widgets", widgets)
	mux.HandleFunc("POST /widgets", createWidget)
	mux.HandleFunc("POST /orders", createOrder)
	_ = http.ListenAndServe(":8080", mux)
}
