// Package main exercises interface-typed REQUEST body resolution (issue #164):
// when a handler decodes into a value whose static type is an interface, the
// request schema should document the concrete type actually assigned to it,
// mirroring what testdata/interface_response already does for responses. When
// more than one concrete type is assigned, the payload is genuinely one of them
// and maps to `oneOf` (issue #201) — honest over wrong, without discarding what
// is known.
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

// createDog: `var a Animal = Dog{}` (declaration with init) → resolves to Dog.
func createDog(w http.ResponseWriter, r *http.Request) {
	var a Animal = Dog{}
	_ = json.NewDecoder(r.Body).Decode(&a)
	w.WriteHeader(http.StatusCreated)
}

// createCat: declared then assigned (`var a Animal; a = Cat{}`) → resolves to Cat.
func createCat(w http.ResponseWriter, r *http.Request) {
	var a Animal
	a = Cat{}
	_ = json.NewDecoder(r.Body).Decode(&a)
	w.WriteHeader(http.StatusCreated)
}

// createEither assigns two different concrete types on different branches, so
// the payload is genuinely one of them — the schema is a `oneOf` of both
// (issue #201), not a guessed single type and not the bare interface.
func createEither(w http.ResponseWriter, r *http.Request) {
	var a Animal = Dog{}
	if r.URL.Query().Get("x") == "1" {
		a = Cat{}
	}
	_ = json.NewDecoder(r.Body).Decode(&a)
	w.WriteHeader(http.StatusCreated)
}

// createConcrete is the baseline: a concrete decode target, which already
// resolved before this change and must keep working.
func createConcrete(w http.ResponseWriter, r *http.Request) {
	var d Dog
	_ = json.NewDecoder(r.Body).Decode(&d)
	w.WriteHeader(http.StatusCreated)
}

// createViaParam passes a concrete value into a helper whose parameter is the
// interface — the param-binding shape (`decodeAnimal(r, Dog{})`).
func createViaParam(w http.ResponseWriter, r *http.Request) {
	decodeAnimal(r, Cat{})
	w.WriteHeader(http.StatusCreated)
}

func decodeAnimal(r *http.Request, a Animal) {
	_ = json.NewDecoder(r.Body).Decode(&a)
}

// createPointer holds a POINTER to the concrete type and decodes into it
// directly (`Decode(a)` rather than `Decode(&a)`) — a shape the response
// fixture never exercises, since responses encode the value itself.
func createPointer(w http.ResponseWriter, r *http.Request) {
	var a Animal = &Dog{}
	_ = json.NewDecoder(r.Body).Decode(a)
	w.WriteHeader(http.StatusCreated)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /dogs", createDog)
	mux.HandleFunc("POST /cats", createCat)
	mux.HandleFunc("POST /either", createEither)
	mux.HandleFunc("POST /concrete", createConcrete)
	mux.HandleFunc("POST /via-param", createViaParam)
	mux.HandleFunc("POST /pointer", createPointer)
	_ = http.ListenAndServe(":8080", mux)
}
