// Fixture: response bodies written as a map composite literal — the usual Go way
// to put a named envelope around a payload (issue #295).
//
// `map[string]any` is all the TYPE says, so mapping from the type alone can only
// ever produce `additionalProperties: {type: object}` and the payload's own
// component is never reached. Both facts are at the literal: the key is a
// constant and the value carries its resolved type.
//
// The routes cover what the literal can and cannot say:
//
//	/envelope  every key constant, values of three different shapes
//	/mixed     one constant key and one computed key — the known key must be
//	           named, the unknown one must stay possible, neither denied
//	/dynamic   no literal at all (a map built at runtime), which is the case
//	           additionalProperties is actually FOR and must still produce
//	/intkey    a non-string key, which is not an OpenAPI object at all
//	/typed     the control: the same payload with no envelope
package main

import (
	"encoding/json"
	"net/http"
)

type CostCode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Meta struct {
	Total int `json:"total"`
}

func envelope(w http.ResponseWriter, r *http.Request) {
	items := []CostCode{{ID: "1", Name: "a"}}
	meta := Meta{Total: 1}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"cost_codes": items,
		"meta":       meta,
		"cursor":     "abc",
	})
}

func mixed(w http.ResponseWriter, r *http.Request) {
	items := []CostCode{{ID: "1", Name: "a"}}
	key := r.URL.Query().Get("group")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"cost_codes": items,
		key:          "grouped",
	})
}

func dynamic(w http.ResponseWriter, r *http.Request) {
	out := map[string]CostCode{}
	for _, c := range []CostCode{{ID: "1"}} {
		out[c.ID] = c
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func intkey(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[int]string{1: "a"})
}

func typed(w http.ResponseWriter, r *http.Request) {
	items := []CostCode{{ID: "1", Name: "a"}}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(items)
}

func main() {
	http.HandleFunc("/envelope", envelope)
	http.HandleFunc("/mixed", mixed)
	http.HandleFunc("/dynamic", dynamic)
	http.HandleFunc("/intkey", intkey)
	http.HandleFunc("/typed", typed)
	_ = http.ListenAndServe(":8080", nil)
}
