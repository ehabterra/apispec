// Package main exercises where a parameter's NAME comes from.
//
// A name written as a literal has always resolved. Everything else here is a
// normal way to write the same thing — a local variable, a package constant,
// a constant from another package — and each one that can be resolved must
// produce the same parameter as the literal does (issue #453).
//
// The last two are the honest-failure cases: a name that genuinely cannot be
// evaluated, and one with two disagreeing assignments. Neither may be guessed;
// both must be left out of the spec entirely (golden rule #7). A parameter
// emitted with the wrong name is worse than one that is missing, and a
// parameter emitted with NO name is invalid OpenAPI (issue #452).
package main

import (
	"net/http"

	"paramnamesources/hdr"
)

// headerRequestID is the shape most real code uses for a header name.
const headerRequestID = "X-Request-ID"

// headerTrace is a package-level variable rather than a constant.
var headerTrace = "X-Trace"

type headerKind string

func main() {
	mux := http.NewServeMux()

	// Control: a literal, which already worked.
	mux.HandleFunc("GET /literal", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("X-Literal")
	})

	// A local variable holding the name.
	mux.HandleFunc("GET /localvar", func(w http.ResponseWriter, r *http.Request) {
		key := "X-Local"
		_ = r.Header.Get(key)
	})

	// A constant declared in this package.
	mux.HandleFunc("GET /pkgconst", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(headerRequestID)
	})

	// A variable declared in this package.
	mux.HandleFunc("GET /pkgvar", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(headerTrace)
	})

	// A constant from another package.
	mux.HandleFunc("GET /crosspkg", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(hdr.Name)
	})

	// A field of a package-level struct value.
	mux.HandleFunc("GET /structfield", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(hdr.Config.Key)
	})

	// A query parameter, to show this is not header-specific.
	mux.HandleFunc("GET /queryvar", func(w http.ResponseWriter, r *http.Request) {
		page := "page"
		_ = r.URL.Query().Get(page)
	})

	// Through a type conversion, which changes the type and not the string.
	mux.HandleFunc("GET /conversion", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(string(headerKind("X-Converted")))
	})

	// The name is passed in by a wrapper.
	mux.HandleFunc("GET /viaparam", func(w http.ResponseWriter, r *http.Request) {
		_ = readHeader(r, "X-Via-Param")
	})

	// Honest failure: not evaluable at all.
	mux.HandleFunc("GET /unresolvable", func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get(hdr.Dynamic())
	})

	// Honest failure: two assignments that disagree.
	mux.HandleFunc("GET /ambiguous", func(w http.ResponseWriter, r *http.Request) {
		key := "X-One"
		if r.URL.RawQuery != "" {
			key = "X-Two"
		}
		_ = r.Header.Get(key)
	})

	_ = http.ListenAndServe(":8080", mux)
}

func readHeader(r *http.Request, name string) string {
	return r.Header.Get(name)
}
