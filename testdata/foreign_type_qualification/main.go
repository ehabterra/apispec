// Package main covers how a type gets qualified when it is referenced from a
// package other than the one declaring it (issue #329).
//
// A map-literal response resolves each value's type through the argument
// renderer's qualification tail. A short qualified name like time.Time has no
// slash in it, so a "does this contain /" test read it as unqualified and
// re-attached the package the argument appears in — producing
// `<this package>-->time.Time`, which matches neither metadata nor the
// well-known external types, so a timestamp documented as an empty object.
package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// Local is declared here, so it must still be qualified with this package.
type Local struct {
	Name string `json:"name"`
}

func status(w http.ResponseWriter, r *http.Request) {
	// The map-literal path: each value's type is resolved separately.
	checkedAt := time.Now()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"checked_at": checkedAt,
		"uptime":     time.Since(time.Now()),
		"local":      Local{Name: "x"},
		"ok":         true,
	})
}

// Event carries a foreign type as a struct field, the other route into the
// same qualification.
type Event struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Local     Local     `json:"local"`
}

func event(w http.ResponseWriter, r *http.Request) {
	// The field is assigned from a VARIABLE, so the type is resolved through
	// the argument's own package rather than read off the field declaration —
	// the shape that triggered the mis-qualification.
	now := time.Now()
	_ = json.NewEncoder(w).Encode(Event{ID: "e1", CreatedAt: now})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", status)
	mux.HandleFunc("GET /events/{id}", event)
	_ = http.ListenAndServe(":8080", mux)
}
