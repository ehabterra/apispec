// json.RawMessage is bytes copied into the document verbatim, so it is ANY JSON
// value. The marshaler fallback called it a string — the one answer that is
// almost never right, since a validator would then reject every real payload
// (issue #518).
//
// The three forms sit together because each reaches the schema by a different
// branch of the mapper: the named type, the pointer branch, and the slice
// branch that recurses into its element.
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Envelope struct {
	Name string `json:"name"`
	// Any JSON value.
	Raw json.RawMessage `json:"raw"`
	// The pointer branch, which returns the pointee's schema.
	OptRaw *json.RawMessage `json:"optRaw"`
	// The slice branch, which recurses into the element.
	ManyRaw []json.RawMessage `json:"manyRaw"`
	// A type whose JSON form IS knowable stays precise, so the entry does not
	// turn every marshaler into "any".
	When time.Time `json:"when"`
}

func getEnvelope(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Envelope{})
}

func createEnvelope(w http.ResponseWriter, r *http.Request) {
	var in Envelope
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func main() {
	r := chi.NewRouter()
	r.Get("/envelopes", getEnvelope)
	r.Post("/envelopes", createEnvelope)
	_ = http.ListenAndServe(":8080", r)
}
