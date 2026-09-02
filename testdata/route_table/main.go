// Package main registers routes from a TABLE, whose method and path exist only
// as runtime values (issue #428). Such a registration is reported and left out
// of the document: it used to be emitted under the placeholder standing in for
// the expression — `/{Method} {Path}` — which is an operation no request can
// match and a client method for an endpoint that does not exist.
//
// The literal and PARTLY-resolved registrations alongside it must still be
// documented: an unresolved prefix leaves a real endpoint whose location is
// approximate, which is worth keeping with the placeholder flagged (#274/#34).
//
// The variant where such a table is registered on a sub-mux MOUNTED at a
// literal prefix — dropped for the other reason, since `/api/{Method} {Path}`
// is not a valid path template whatever the prefix — is covered by a unit test
// instead (TestUndocumentablePath). Two tables in one fixture render the same
// handler identity (`route.Handler`, the field read), so they conflate into one
// route and the fixture would be a change detector for that instead.
package main

import (
	"encoding/json"
	"net/http"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]User{})
}

func getUser(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func partly(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

type route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// routes is the table: nothing here is readable from the registration site.
var routes = []route{
	{"GET", "/users", listUsers},
	{"GET", "/users/{id}", getUser},
}

// prefix is evaluated at runtime, so the path it contributes is a placeholder —
// but the rest of the path is written literally.
func prefix() string {
	return "/api"
}

func main() {
	mux := http.NewServeMux()

	// Dropped: neither the verb nor the path can be read here.
	for _, rt := range routes {
		mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)
	}

	// Kept: written literally.
	mux.HandleFunc("GET /health", health)

	// Kept: the prefix is a placeholder, the tail is real.
	mux.HandleFunc("GET "+prefix()+"/partly", partly)

	http.ListenAndServe(":8080", mux)
}
