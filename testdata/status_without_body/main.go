// Fixture: a response with no body must carry no `content` block.
//
// A status the handler writes without ever writing a body was still emitted as
// `content: {application/json: {}}` — an empty media-type object, which says
// "there IS a JSON body, and I cannot describe it". That is a different claim
// from "there is no body", and it is the wrong one: a client generator reads it
// as an unknown-shaped payload and a validator sees a body where none is sent.
//
// The distinction the mapper has to keep is between a response with no body at
// all (here) and one whose body was seen but could not be typed, which keeps
// its content block. See internal/spec/mapper_test.go.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /forbidden", forbidden)
	mux.HandleFunc("GET /teapot", teapot)
	mux.HandleFunc("GET /item", item)
	http.ListenAndServe(":8080", mux)
}

// forbidden writes a status and nothing else: no body, so no content.
func forbidden(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusForbidden)
}

// teapot is the same shape at another status, to prove nothing is special-cased
// about the code itself.
func teapot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTeapot)
}

// item is the control: a real body, which must keep its schema.
func item(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}
