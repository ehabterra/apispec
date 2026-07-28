// Package main registers routes from two packages through the same router.
//
// Both packages make the SAME chi calls, so nothing about the call itself
// separates them — only the package the call is made FROM. A spec that should
// describe the public API and not the operator endpoints needs to say so with
// callerPkgPatterns; without it, both sets are documented (issue #238).
package main

import (
	"net/http"

	"example.com/callerscope/internal/api"
	"example.com/callerscope/internal/debugroutes"
	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	api.Register(r)
	debugroutes.Register(r)
	_ = http.ListenAndServe(":8080", r)
}
