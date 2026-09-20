// A response whose value is an INLINE struct type literal lost its body: the
// operation documented a status with no content, while the identical value
// assigned to a variable first resolved perfectly (issue #515).
//
// The variable-vs-inline split golden rule #11 warns about for routers, showing
// up for values. A variable of an anonymous struct type gets a synthetic type
// registered for it and an ident that names it; the literal had neither, so it
// reached the mapper as a bare composite whose fields live on a `struct_type`
// node nothing reads. A map literal through the same helper always worked,
// because a map IS described by its type expression.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

type Meta struct {
	Name string `json:"name"`
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// A: the inline literal through a helper — the shape that was lost.
func inlineThroughHelper(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, 200, struct {
		Values    []string `json:"values"`
		Truncated bool     `json:"truncated"`
	}{Values: nil})
}

// B: the identical value, assigned first. This always worked, and is the
// control that says the two spellings must agree.
func assignedFirst(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Values    []string `json:"values"`
		Truncated bool     `json:"truncated"`
	}{Values: nil}
	respondJSON(w, 200, out)
}

// C: the inline literal encoded directly. Its status came out as `default`
// too — a symptom of the missing body, not a second defect: the implicit-200
// rule (#369) needs a body to apply to.
func inlineDirect(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(struct {
		Values []string `json:"values"`
	}{})
}

// D: a map literal through the same helper — the control that always resolved.
func mapLiteral(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, 200, map[string]any{"values": []string{}})
}

// One shared error helper for every handler below, each with a different
// success body. This is the shape a real service writes, and the one that made
// the fix worth checking past the four cases above: an inline error envelope
// reached from three routes has to land on each route's OWN status, and must
// not displace a success body that is already there.
func apiError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Errors []string `json:"errors"`
	}{Errors: []string{msg}})
}

func sharedMeta(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("n") == "" {
		apiError(w, http.StatusNotFound, "nope")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Meta{Name: "x"})
}

func sharedDownload(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open("/tmp/x.pdf")
	if err != nil {
		apiError(w, http.StatusInternalServerError, "boom")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = io.Copy(w, f)
}

func main() {
	r := chi.NewRouter()
	r.Get("/inline", inlineThroughHelper)
	r.Get("/assigned", assignedFirst)
	r.Get("/direct", inlineDirect)
	r.Get("/map", mapLiteral)
	r.Get("/shared-meta", sharedMeta)
	r.Get("/shared-download.pdf", sharedDownload)
	_ = http.ListenAndServe(":8080", r)
}
