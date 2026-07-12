// Package main exercises interface-typed response resolution: when a handler
// encodes a value whose static type is an interface, the schema should document
// the concrete type actually assigned to it (traceable statically), not the
// empty interface. When more than one concrete type is assigned, resolution is
// ambiguous and the interface is kept (honest over wrong).
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
// concrete type is ambiguous — resolution keeps the Animal interface.
func getEither(w http.ResponseWriter, r *http.Request) {
	var a Animal = Dog{}
	if r.URL.Query().Get("x") == "1" {
		a = Cat{}
	}
	json.NewEncoder(w).Encode(a)
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
	http.ListenAndServe(":8080", nil)
}
