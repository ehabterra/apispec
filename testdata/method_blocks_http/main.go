// Fixture: control flow inside a METHOD handler answers exactly as it does
// inside the identical FUNCTION handler (issue #617).
//
// Metadata used to record control-flow regions for declared functions only, so
// every query inside a method body answered on missing facts: a status write
// "dominated" everything below it, and no two writes were ever alternatives.
// Each shape below is written twice — /fn/... served by a function, /m/... by a
// method on H — and the two must document the same responses. See
// generator/testdata_method_blocks_test.go.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

type Err struct {
	Message string `json:"message"`
}

type H struct{}

func main() {
	mux := http.NewServeMux()
	h := &H{}

	mux.HandleFunc("POST /fn/create", create)
	mux.HandleFunc("POST /m/create", h.Create)

	mux.HandleFunc("GET /fn/guard", guard)
	mux.HandleFunc("GET /m/guard", h.Guard)

	mux.HandleFunc("GET /fn/alternatives", alternatives)
	mux.HandleFunc("GET /m/alternatives", h.Alternatives)

	mux.HandleFunc("GET /fn/sequential", sequential)
	mux.HandleFunc("GET /m/sequential", h.Sequential)

	mux.HandleFunc("GET /fn/branchstatus", branchStatus)
	mux.HandleFunc("GET /m/branchstatus", h.BranchStatus)

	http.ListenAndServe(":8080", mux)
}

// create is the early-return error shape: the 400 and its body sit in an arm
// that returns, so the 201 below is reached only when the 400 did not run.
func create(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("bad") != "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(Err{Message: "bad"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

func (h *H) Create(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("bad") != "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(Err{Message: "bad"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

// guard's 400 is bodyless: the success body below must not carry it.
func guard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

func (h *H) Guard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

// alternatives writes one status above a branch whose arms send different
// types: the 202 really is either, so it is their alternation (#391).
func alternatives(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	if r.URL.Query().Get("detail") != "" {
		_ = json.NewEncoder(w).Encode(Item{ID: "1"})
		return
	}
	_ = json.NewEncoder(w).Encode(Err{Message: "queued"})
}

func (h *H) Alternatives(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	if r.URL.Query().Get("detail") != "" {
		_ = json.NewEncoder(w).Encode(Item{ID: "1"})
		return
	}
	_ = json.NewEncoder(w).Encode(Err{Message: "queued"})
}

// sequential writes two bodies one after the other under one status. They are
// not alternatives — both run — so the second must not be folded into the
// 201 as an anyOf member.
func sequential(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
	_ = json.NewEncoder(w).Encode(Err{Message: "trailer"})
}

func (h *H) Sequential(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
	_ = json.NewEncoder(w).Encode(Err{Message: "trailer"})
}

// branchStatus picks its status in a branch: the 200 written above the `if`
// still reaches the write whenever the branch is not taken, so it must not be
// read as overwritten by the 202 inside it.
func branchStatus(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	if r.URL.Query().Get("async") != "" {
		status = http.StatusAccepted
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

func (h *H) BranchStatus(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	if r.URL.Query().Get("async") != "" {
		status = http.StatusAccepted
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}
