// encoding/json already states which fields are always on the wire: one with no
// `omitempty` and no `omitzero` is written on every encode. None of that reached
// the document, so a generated TypeScript client null-checked every field —
// 2,233 of 2,900 properties on the service that reported it (issue #516).
//
// Presence only. Whether a value may be null is a separate statement and
// belongs to #368: `Ptr` below is always PRESENT, written as `null`, so it is
// required here and nullable there.
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Base is embedded by VALUE: its fields are always there.
type Base struct {
	ID   string `json:"id"`
	Note string `json:"note,omitempty"`
}

// Meta is embedded by POINTER: its fields vanish entirely when it is nil,
// however their own tags read.
type Meta struct {
	Trace string `json:"trace"`
}

type Item struct {
	Base
	*Meta

	Name string `json:"name"`
	// Absent when empty.
	Optional string `json:"optional,omitempty"`
	// Absent when zero (Go 1.24).
	Zeroable string `json:"zeroable,omitzero"`
	// Always present, written as null when nil.
	Ptr *string `json:"ptr"`
	// Absent when nil.
	PtrOpt *string `json:"ptrOpt,omitempty"`
	// Never on the wire at all.
	Hidden string `json:"-"`
	// Mandatory on the way IN. The two statements are merged, not duplicated.
	Validated string `json:"validated" validate:"required"`
	// An external type is still a field like any other.
	When time.Time `json:"when"`
	// A pointer to a NAMED type: a $ref may carry no sibling keywords, so the
	// union is the only encoding available for it.
	Owner *Base `json:"owner"`
	// A plain slice — nil until something is appended, and written as `null`
	// then. This is the shape every list response has, and the one a
	// pointers-only rule misses; `*[]string` is a double indirection almost
	// nobody writes, because a slice is already nilable.
	Tags []string `json:"tags"`
	// A nil map is null too.
	Labels map[string]string `json:"labels"`
	// So is an interface holding nothing.
	Extra any `json:"extra"`
	// A fixed-size array CANNOT be nil: it is written as an array of zero
	// values, so it is never widened. The case that makes prefix-matching on
	// "[" wrong.
	Pair [2]string `json:"pair"`
	// Nilable, but told to disappear instead of going null.
	Skipped []string `json:"skipped,omitempty"`
}

// Money marshals ITSELF, so its declared fields are not what reaches the wire —
// nothing about their tags is a statement about the document.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) { return []byte(`"0.00 USD"`), nil }

type Invoice struct {
	Total Money  `json:"total"`
	Ref   string `json:"ref"`
}

func getInvoice(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Invoice{})
}

func getItem(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

func createItem(w http.ResponseWriter, r *http.Request) {
	var in Item
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func main() {
	r := chi.NewRouter()
	r.Get("/items", getItem)
	r.Get("/invoices", getInvoice)
	r.Post("/items", createItem)
	_ = http.ListenAndServe(":8080", r)
}
