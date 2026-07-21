// Package main exercises interface-typed response resolution: when a handler
// encodes a value whose static type is an interface, the schema should document
// the concrete type actually assigned to it (traceable statically), not the
// empty interface. When more than one concrete type is assigned, the payload is
// genuinely one of them and maps to `oneOf` (issue #201) — honest over wrong,
// without discarding what is known.
package main

import (
	"encoding/json"
	"net/http"
)

type Cat struct {
	Name  string `json:"name"`
	Lives int    `json:"lives"`
}

type Dog struct {
	Name  string `json:"name"`
	Breed string `json:"breed"`
}

// Animal is an interface; both Cat and Dog implement it.
type Animal interface{ Sound() string }

func (Cat) Sound() string { return "meow" }
func (Dog) Sound() string { return "woof" }

// getDog: `var a Animal = Dog{}` (declaration with init) → resolves to Dog.
func getDog(w http.ResponseWriter, r *http.Request) {
	var a Animal = Dog{}
	json.NewEncoder(w).Encode(a)
}

// getCat: declared then assigned (`var a Animal; a = Cat{}`) → resolves to Cat.
func getCat(w http.ResponseWriter, r *http.Request) {
	var a Animal
	a = Cat{}
	json.NewEncoder(w).Encode(a)
}

// getEither assigns two different concrete types on different branches, so the
// payload is genuinely one of them — the schema is a `oneOf` of both
// (issue #201), not a guessed single type and not the bare interface.
func getEither(w http.ResponseWriter, r *http.Request) {
	var a Animal = Dog{}
	if r.URL.Query().Get("x") == "1" {
		a = Cat{}
	}
	json.NewEncoder(w).Encode(a)
}

// writeAnimal encodes a value received through a named-interface parameter.
func writeAnimal(w http.ResponseWriter, v Animal) { json.NewEncoder(w).Encode(v) }

// getPassed passes a concrete Dog into a helper whose parameter is the Animal
// interface; resolution traces the param back to the call-site concrete.
func getPassed(w http.ResponseWriter, r *http.Request) {
	writeAnimal(w, Dog{})
}

// makeDog returns a concrete Dog through a function whose declared return type
// is the Animal interface.
func makeDog() Animal { return Dog{} }

// getMade encodes the result of a constructor typed to return the interface;
// resolution traces the callee's return value to the concrete Dog.
func getMade(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(makeDog())
}

func main() {
	http.HandleFunc("/dog", getDog)
	http.HandleFunc("/cat", getCat)
	http.HandleFunc("/either", getEither)
	http.HandleFunc("/made", getMade)
	http.HandleFunc("/passed", getPassed)
	http.ListenAndServe(":8080", nil)
}
