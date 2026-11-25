package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type Problem struct {
	Title string `json:"title"`
}

type Item struct {
	ID string `json:"id"`
}

// problem declares the media type the encoder's bytes go out under. net/http's
// encoder writes no Content-Type of its own, so the declaration IS the body's.
func problem(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fnHelper(w http.ResponseWriter, _ *http.Request) {
	problem(w, http.StatusBadRequest, Problem{Title: "bad"})
}

type responder struct{}

func (responder) problem(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// late declares after WriteHeader has committed the headers: net/http ignores
// the write, so the body goes out under the encoder's default, not the
// declaration.
func (responder) late(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/problem+json")
	_ = json.NewEncoder(w).Encode(v)
}

// negotiated declares a different media type on each arm. Neither declaration
// is in effect on the other arm's body, so neither is the responder's.
func (responder) negotiated(w http.ResponseWriter, legacy bool, status int, v any) {
	if legacy {
		w.Header().Set("Content-Type", "application/vnd.legacy+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type api struct{ out responder }

func (a *api) methodHelper(w http.ResponseWriter, _ *http.Request) {
	a.out.problem(w, http.StatusNotFound, Problem{Title: "missing"})
}

func (a *api) lateHelper(w http.ResponseWriter, _ *http.Request) {
	a.out.late(w, http.StatusGone, Problem{Title: "gone"})
}

func (a *api) negotiatedHelper(w http.ResponseWriter, r *http.Request) {
	a.out.negotiated(w, r.URL.Query().Get("legacy") != "", http.StatusPreconditionFailed, Problem{Title: "stale"})
}

func inline(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(Problem{Title: "conflict"})
}

// No status write: the body goes out under the implicit 200, and the
// declaration is still its media type.
func implicitStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.item+json")
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

// The declaration is made on one branch only: on the other the body goes out
// under the default media type, so it is not the body's media type.
func branchDeclared(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("problem") != "" {
		w.Header().Set("Content-Type", "application/problem+json")
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(Problem{Title: "invalid"})
}

// writeJSON is shared: declaredJSON reaches it after declaring a media type,
// plainJSON reaches it directly and declares nothing.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func declaredJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, http.StatusForbidden, Problem{Title: "forbidden"})
}

func plainJSON(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusAccepted, Item{ID: "2"})
}

// render.JSON sets Content-Type to application/json itself, replacing the
// declaration: the error body goes out as JSON, not as the CSV declared ahead.
func renderJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	if r.URL.Query().Get("fail") != "" {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, Problem{Title: "export failed"})
		return
	}
	_, _ = w.Write([]byte("id\n1\n"))
}

func main() {
	r := chi.NewRouter()
	a := &api{}
	r.Get("/fn-helper", fnHelper)
	r.Get("/method-helper", a.methodHelper)
	r.Get("/late-helper", a.lateHelper)
	r.Get("/negotiated-helper", a.negotiatedHelper)
	r.Get("/inline", inline)
	r.Get("/implicit-status", implicitStatus)
	r.Get("/branch-declared", branchDeclared)
	r.Get("/declared-json", declaredJSON)
	r.Get("/plain-json", plainJSON)
	r.Get("/render-json", renderJSON)
	_ = http.ListenAndServe(":8080", r)
}
