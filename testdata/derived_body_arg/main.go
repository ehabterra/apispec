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

// bindOrReject decodes into the caller's destination and rejects a bad body
// with a message built from the request. The rejection has no status
// parameter and a body the method makes, so it contributes nothing to the
// derived pattern — and must not take the decoder role down with it.
func (rt *R) bindOrReject(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "bad body for "+r.URL.Path, http.StatusBadRequest)
		return false
	}
	return true
}

// rejectOrBind is bindOrReject with its two calls in the other order, so the
// rejection reaches the derivation before the decode does.
func (rt *R) rejectOrBind(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		http.Error(w, "empty body for "+r.URL.Path, http.StatusBadRequest)
		return false
	}
	return json.NewDecoder(r.Body).Decode(dst) == nil
}

// sendWrapped reassigns its parameter before encoding it: the body is the map
// the method builds around the caller's value, not that value.
func (rt *R) sendWrapped(w http.ResponseWriter, v any) {
	v = map[string]any{"data": v}
	_ = json.NewEncoder(w).Encode(v)
}

// reject builds its message from the request and forwards its status: the
// status is the caller's, the body is not.
func (rt *R) reject(w http.ResponseWriter, req *http.Request, code int) {
	http.Error(w, "rejected "+req.URL.Path, code)
}

// deny forwards to reject unchanged. It writes nothing itself, so whether its
// body is the caller's is whatever reject already established: it is not.
func (rt *R) deny(w http.ResponseWriter, req *http.Request, code int) {
	rt.reject(w, req, code)
}

// Envelope wraps every payload bindEnvelope reads.
type Envelope struct {
	Data any `json:"data"`
}

// Order is the payload handed to bindEnvelope.
type Order struct {
	Qty int `json:"qty"`
}

// Gadget is the payload handed to bindOrReject.
type Gadget struct {
	Serial string `json:"serial"`
}

// Part is the payload handed to rejectOrBind.
type Part struct {
	Code string `json:"code"`
}

// Receipt is the value sendWrapped wraps.
type Receipt struct {
	Total int `json:"total"`
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

func createGadget(w http.ResponseWriter, r *http.Request) {
	var in Gadget
	if !(&R{}).bindOrReject(w, r, &in) {
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func createPart(w http.ResponseWriter, r *http.Request) {
	var in Part
	if !(&R{}).rejectOrBind(w, r, &in) {
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func receipts(w http.ResponseWriter, r *http.Request) {
	(&R{}).sendWrapped(w, Receipt{})
}

func denied(w http.ResponseWriter, r *http.Request) {
	rt := &R{}
	if r.URL.Query().Get("token") == "" {
		rt.deny(w, r, http.StatusForbidden)
		return
	}
	rt.send(w, Item{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /normalized", normalized)
	mux.HandleFunc("GET /rejected", rejected)
	mux.HandleFunc("GET /widgets", widgets)
	mux.HandleFunc("POST /widgets", createWidget)
	mux.HandleFunc("POST /orders", createOrder)
	mux.HandleFunc("POST /gadgets", createGadget)
	mux.HandleFunc("POST /parts", createPart)
	mux.HandleFunc("GET /receipts", receipts)
	mux.HandleFunc("GET /denied", denied)
	_ = http.ListenAndServe(":8080", mux)
}
