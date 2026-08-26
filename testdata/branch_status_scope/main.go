// Fixture: a status write only claims a body the write DOMINATES.
//
// Response pairing walks fragments in source order, so a status written above a
// body used to claim it whatever the control flow between them. A guard clause
// is the shape that breaks on: `if bad { WriteHeader(400); return }` sits above
// the success body, but a request that reaches that body is precisely one where
// the 400 did NOT run. The endpoint ended up documented with its error status
// carrying its success schema, and with no success response at all.
//
// Each handler here isolates one relation between the status write and the body
// write. See generator/testdata_branch_status_scope_test.go.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type APIError struct {
	Message string `json:"message"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /guard/{id}", guardBodyless)
	mux.HandleFunc("GET /paired/{id}", pairedInArm)
	mux.HandleFunc("GET /nested/{id}", nestedInArm)
	mux.HandleFunc("GET /loop", statusInLoop)
	mux.HandleFunc("GET /arms", statusInEveryArm)
	mux.HandleFunc("GET /before", statusBeforeBranch)
	mux.HandleFunc("GET /alternatives", statusOverTwoTypes)

	http.ListenAndServe(":8080", mux)
}

// guardBodyless is the shape this fixture exists for: the 400 is written in a
// branch that returns, so the body below it is reached only when the 400 did
// not run. The 400 is bodyless and the success body is an implicit 200.
func guardBodyless(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// pairedInArm is the control: the body is written INSIDE the arm that set the
// status, so it does carry it. Nothing about this case may change.
func pairedInArm(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIError{Message: "id is required"})
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// nestedInArm writes the body deeper inside the arm that set the status. Every
// path to the body still passes the status write, so it is paired — nesting is
// not escaping.
func nestedInArm(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") == "" {
		w.WriteHeader(http.StatusBadRequest)
		if r.URL.Query().Get("verbose") != "" {
			_ = json.NewEncoder(w).Encode(APIError{Message: "id is required"})
		}
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// statusInLoop writes the status inside a loop, which may run zero times, so
// the body below it is not dominated by the write.
func statusInLoop(w http.ResponseWriter, r *http.Request) {
	for _, v := range r.URL.Query()["reset"] {
		if v == "1" {
			w.WriteHeader(http.StatusAccepted)
		}
	}
	_ = json.NewEncoder(w).Encode([]Item{{ID: "1"}})
}

// statusInEveryArm states a different status on each arm and writes the body
// below the branch. No single write dominates the body, so which status it
// carries is genuinely undetermined — the honest answer is neither of them
// rather than whichever the walk saw last (golden rule #7).
func statusInEveryArm(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("async") != "" {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusPartialContent)
	}
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

// statusOverTwoTypes writes one status above a branch whose arms send
// DIFFERENT types. The endpoint really can answer 202 with either, so the
// response is the alternation of them rather than whichever the walk saw last.
func statusOverTwoTypes(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	if r.URL.Query().Get("detail") != "" {
		_ = json.NewEncoder(w).Encode(Item{ID: "1", Name: "detailed"})
		return
	}
	_ = json.NewEncoder(w).Encode(APIError{Message: "queued"})
}

// statusBeforeBranch writes the status unconditionally, then a body on each
// side of a branch. Every path to both bodies passed the write, so both are
// 201s — and both bodies are the same type, so the response is one schema
// (issue #391).
func statusBeforeBranch(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	if r.URL.Query().Get("full") != "" {
		_ = json.NewEncoder(w).Encode(Item{ID: "1", Name: "full"})
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}
