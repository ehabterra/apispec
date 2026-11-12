// Package main binds whole structs with gorilla/schema, the decoder the
// net/http-based routers use (issue #602). Decode reads whatever map it is
// handed, so the location comes from the source: the request's query is query
// parameters, its parsed form is a form body, and anything else is not a
// request read at all.
package main

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gorilla/schema"
)

type Item struct {
	ID string `json:"id"`
}

type ListQuery struct {
	Page   int    `schema:"page,required"`
	Limit  int    `schema:"limit,default:20"`
	Sort   string `schema:"sort"`
	Cursor string // untagged: gorilla/schema binds it under the Go name
	Skip   string `schema:"-"`
}

type SearchQuery struct {
	Q string `schema:"q" validate:"required"`
}

type WrappedQuery struct {
	Tag []string `schema:"tag"`
}

type CreateForm struct {
	Name  string `schema:"name,required"`
	Email string `schema:"email"`
}

type UpstreamQuery struct {
	Token string `schema:"token"`
}

var decoder = schema.NewDecoder()

// upstreamURL is configuration, not the request.
var upstreamURL = "https://upstream.example/api?token=abc"

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// listItems decodes the request's query directly.
func listItems(w http.ResponseWriter, r *http.Request) {
	var q ListQuery
	if err := decoder.Decode(&q, r.URL.Query()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, []Item{})
}

// search decodes the query through a local variable.
func search(w http.ResponseWriter, r *http.Request) {
	vals := r.URL.Query()
	var q SearchQuery
	if err := decoder.Decode(&q, vals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, []Item{})
}

// decodeQuery is a house wrapper: the source and the type are the caller's.
func decodeQuery(dst any, src url.Values) error {
	return decoder.Decode(dst, src)
}

func tagged(w http.ResponseWriter, r *http.Request) {
	var q WrappedQuery
	if err := decodeQuery(&q, r.URL.Query()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, []Item{})
}

// createItem decodes the parsed form body.
func createItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var f CreateForm
	if err := decoder.Decode(&f, r.PostForm); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, Item{})
}

// upstream decodes the query of a URL built from configuration: no request
// parameter is read, so nothing is documented.
func upstream(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var q UpstreamQuery
	_ = decoder.Decode(&q, u.Query())
	writeJSON(w, Item{})
}

// defaultQuery is configuration held in a package-level variable.
var defaultQuery = url.Values{"token": {"abc"}}

// defaults decodes that package-level map: it is set at start-up, never per
// request, so no request parameter is read.
func defaults(w http.ResponseWriter, r *http.Request) {
	var q UpstreamQuery
	_ = decoder.Decode(&q, defaultQuery)
	writeJSON(w, Item{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", listItems)
	mux.HandleFunc("POST /items", createItem)
	mux.HandleFunc("GET /search", search)
	mux.HandleFunc("GET /tagged", tagged)
	mux.HandleFunc("GET /upstream", upstream)
	mux.HandleFunc("GET /defaults", defaults)
	_ = http.ListenAndServe(":8080", mux)
}
