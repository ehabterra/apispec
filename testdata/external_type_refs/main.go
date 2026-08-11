// Package main exercises response/request types whose fields come from
// packages outside the analysed module. Their declarations are not in metadata,
// so a $ref to them can never be satisfied (issue #325).
package main

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"text/template"
	"time"
)

// Account has fields of types declared outside this module: the standard
// library is not part of the analysed package set, exactly like a dependency.
type Account struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Balance   *big.Int   `json:"balance"`
	Query     url.Values `json:"query"`
	Nested    Inner      `json:"nested"`
	// Named external types of several shapes: a map type, a struct, and one
	// behind a pointer.
	Pattern  *regexp.Regexp     `json:"pattern"`
	Template *template.Template `json:"template"`
	Loc      *time.Location     `json:"loc"`
	User     *url.Userinfo      `json:"user"`
}

// Inner is declared here, so it must still resolve to a real component.
type Inner struct {
	Label string `json:"label"`
}

// Embedded embeds an external type, the shape gitea hits with
// jwt.RegisteredClaims.
type Embedded struct {
	url.Userinfo
	Name string `json:"name"`
}

func getAccount(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Account{ID: "a1"})
}

func postEmbedded(w http.ResponseWriter, r *http.Request) {
	var in Embedded
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(in)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts/{id}", getAccount)
	mux.HandleFunc("POST /embedded", postEmbedded)
	_ = http.ListenAndServe(":8080", mux)
}
