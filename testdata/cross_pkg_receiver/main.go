// Package main pins two shapes that lost their receiver in the recorded call
// graph (issue #249):
//
//  1. a method on a service held in a struct FIELD of another package — the call
//     was recorded with no receiver at all, under the CALLER's package;
//  2. a call through a field whose declared type is an INLINE interface — the
//     receiver was recorded as the literal string "interface";
//  3. a call through a field typed by a LOCAL ALIAS of a type in another package
//     — the receiver resolved in the caller's package instead of the declaring
//     one.
//
// Both make a pattern scoped by recvTypeRegex unmatchable, and both are
// invisible in a single-package fixture.
package main

import (
	"encoding/json"
	"net/http"

	"example.com/recv/internal/curriculum"
	"example.com/recv/internal/storage"
	"github.com/go-chi/chi/v5"
)

// curriculumAlias is a local alias of a type declared elsewhere, the shape a
// project uses to keep a struct from naming another package at the type level.
type curriculumAlias = curriculum.Service

type Deps struct {
	Curriculum *curriculum.Service

	// Typed by the local alias rather than by the declaring package.
	Aliased *curriculumAlias

	// Declared inline, so the field's type has no name.
	Assets interface {
		Describe() storage.Asset
	}
}

func main() {
	deps := Deps{
		Curriculum: &curriculum.Service{},
		Aliased:    &curriculum.Service{},
		Assets:     &storage.S3{},
	}
	r := chi.NewRouter()
	r.Get("/subjects", listSubjects(deps))
	r.Get("/asset", describeAsset(deps))
	r.Get("/aliased", listAliased(deps))
	_ = http.ListenAndServe(":8080", r)
}

func listSubjects(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := deps.Curriculum.ListSubjects()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}

func listAliased(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := deps.Aliased.ListSubjects()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}

func describeAsset(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := deps.Assets.Describe()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}
