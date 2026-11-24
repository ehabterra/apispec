// A response-writing wrapper handed a value produced in a shared helper (issue
// #615): its response belongs to the routes that call it, never to every route
// that merely reaches the helper producing the value.
//
// `c.Fail(err)` binds getItem's `err` to Fail's parameter, and `err` traces to
// the call that produced it inside findItem. The binding argument hangs under
// that producer, so listItems — which calls findItem but never Fail — reaches
// it too. Read as the Fail call it belongs to, it documented Fail's body as a
// JSON alternative of listItems' plain-text 200. listItems is written first,
// with its status write ahead of the shared call, which is the order that let
// the stray body claim that 200.
package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Ctx is a house response context, the shape of a framework's own Context.
type Ctx struct{ W http.ResponseWriter }

// JSON writes v as the JSON body under status.
func (c *Ctx) JSON(status int, v any) {
	c.W.Header().Set("Content-Type", "application/json")
	c.W.WriteHeader(status)
	_ = json.NewEncoder(c.W).Encode(v)
}

// ErrBody is the body of every failure.
type ErrBody struct {
	Message string `json:"message"`
}

var errMissing = errors.New("missing")

// Fail maps an error to its response. It reads its parameter (err.Error()),
// which is what binds the caller's `err` to it.
func (c *Ctx) Fail(err error) {
	if errors.Is(err, errMissing) {
		c.JSON(http.StatusNotFound, ErrBody{Message: err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, ErrBody{Message: err.Error()})
}

// Item is a stored record.
type Item struct {
	ID string `json:"id"`
}

// findItem is shared by both routes; the error it returns is produced here,
// by a call outside the module, so tracing the caller's `err` ends at this
// assignment.
func findItem(id string) (*Item, error) {
	var item Item
	err := json.NewDecoder(strings.NewReader(id)).Decode(&item)
	return &item, err
}

// listItems reaches findItem too but never calls Fail: a 200 page of plain
// text is its only response, with no JSON error body beside it.
func listItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	item, _ := findItem("first")
	_, _ = w.Write([]byte(item.ID))
}

// getItem fails through the wrapper: 404 and 500 are its responses.
func getItem(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{W: w}
	item, err := findItem(r.PathValue("id"))
	if err != nil {
		c.Fail(err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", getItem)
	mux.HandleFunc("GET /items", listItems)
	_ = http.ListenAndServe(":8080", mux)
}
