package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Item is a named struct used as the element of a nested slice, so the
// innermost items must be a $ref to a real component.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Table reproduces issue #259: the single-level []string is emitted correctly
// while the nested [][]string used to emit a $ref to a component that was
// never defined.
type Table struct {
	Header []string   `json:"header"`
	Rows   [][]string `json:"rows"`
	Cube   [][][]int  `json:"cube"`
	Grid   [][]Item   `json:"grid"`
}

// Containers covers the neighbouring composite shapes named in the issue:
// a slice of maps and a map of slices, both with anonymous element types.
type Containers struct {
	SliceOfMaps  []map[string]string `json:"sliceOfMaps"`
	MapOfSlices  map[string][]string `json:"mapOfSlices"`
	MapOfNested  map[string][][]int  `json:"mapOfNested"`
	SliceOfItems []map[string]Item   `json:"sliceOfItems"`
}

func getTable(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Table{})
}

func getContainers(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Containers{})
}

func main() {
	r := chi.NewRouter()
	r.Get("/table", getTable)
	r.Get("/containers", getContainers)
	_ = http.ListenAndServe(":8080", r)
}
