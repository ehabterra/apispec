// Package main holds registration paths in VARIABLES (issue #431). A path
// assigned two lines above the registration is statically known, and used to be
// treated as unreadable: reported and left out by #428, or — for a path held
// entirely in one variable — rendered as the variable's TYPE, so `/string`
// appeared in the document as if it were an endpoint.
//
// The shapes are side by side because they must not resolve alike: a value the
// assignments agree on is documented, and one they do not is still reported.
package main

import (
	"encoding/json"
	"net/http"
	"os"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func list(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]User{})
}

func one(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

func create(w http.ResponseWriter, r *http.Request) {
	var in User
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
}

func ambiguous(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func fromCall(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// buildPath is evaluated at runtime, so a path assigned from it stays unknown.
func buildPath() string {
	return "/built/" + os.Getenv("SUFFIX")
}

func main() {
	mux := http.NewServeMux()

	// The whole path in one variable. Rendering the argument would give the
	// variable's type, not its value.
	whole := "/users"
	mux.HandleFunc("GET "+whole, list)

	// A bare variable argument, with no verb to split off.
	bare := "/bare"
	mux.HandleFunc(bare, create)

	// An alias chain.
	first := "/aliased"
	second := first
	mux.HandleFunc("GET "+second, one)

	// A variable prefix with a literal tail: both halves resolve, so the whole
	// path is real rather than approximate.
	prefix := "/api"
	mux.HandleFunc("GET "+prefix+"/items", list)

	// A variable assembled from parts that all resolve.
	root := "/v2"
	joined := root + "/joined"
	mux.HandleFunc("GET "+joined, list)

	// Two branches assign different paths: genuinely ambiguous, so it is
	// reported rather than documented at whichever the walk saw last.
	amb := "/first"
	if os.Getenv("ALT") != "" {
		amb = "/second"
	}
	mux.HandleFunc("GET "+amb, ambiguous)

	// Assigned from a call: unknowable, and still reported.
	built := buildPath()
	mux.HandleFunc("GET "+built, fromCall)

	// CHANGE DETECTOR (issue #436): these two ARE knowable, and are not read
	// today, because assignments are matched by name alone. The inner block
	// re-declares `shadowed`, and `later` is written after its registration —
	// so each registration sees two disagreeing values and keeps a placeholder.
	// Documented approximately rather than wrongly, which is why it is a
	// precision gap and not a correctness one.
	shadowed := "/outer"
	mux.HandleFunc("GET "+shadowed+"/a", list)
	{
		shadowed := "/inner"
		mux.HandleFunc("GET "+shadowed+"/b", list)
	}

	later := "/first"
	mux.HandleFunc("GET "+later+"/c", list)
	later = "/second"
	_ = later

	http.ListenAndServe(":8080", mux)
}
