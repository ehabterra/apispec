// Fixture: a struct field whose type has no schema mapping must not serialize
// as a null property.
//
// `Err error` is the shape that surfaced it (gitea's ErrPushRejected). metadata
// classifies `error` as primitive, so the mapper skipped the $ref branch for
// it, found no case for it either, and returned a nil schema — which was stored
// as `properties: {Err: null}`. That is not a vaguer description than `{}`, it
// is an invalid document: ReDoc dereferences every property and dies with
// "Cannot read properties of null (reading '$ref')".
//
// `error` is an interface, so it is documented like `any`. The complex fields
// have no honest mapping at all — encoding/json refuses to marshal them — so
// they get the empty schema rather than an invented type.
package main

import (
	"encoding/json"
	"net/http"
)

// PushFailure mirrors the shape from the wild: an error field beside ordinary
// ones, all exported and all serialized.
type PushFailure struct {
	Message string `json:"message"`
	StdOut  string `json:"stdout"`
	Err     error  `json:"err"`
}

// Sample carries the other types metadata calls primitive but cannot map,
// including inside a slice and a map: the fallback has to apply where the
// schema is BUILT, or a container stores the nil as `items: null` /
// `additionalProperties: null` one level up.
type Sample struct {
	Name     string                `json:"name"`
	Ratio    complex128            `json:"ratio"`
	Smaller  complex64             `json:"smaller"`
	Series   []complex64           `json:"series"`
	ByName   map[string]complex128 `json:"by_name"`
	Optional *complex64            `json:"optional"`
}

func failure(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(PushFailure{Message: "rejected"})
}

func sample(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Sample{Name: "s"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /failure", failure)
	mux.HandleFunc("GET /sample", sample)
	http.ListenAndServe(":8080", mux)
}
