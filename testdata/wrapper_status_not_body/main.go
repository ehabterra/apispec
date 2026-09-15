// Package main is an error helper whose message is derived FROM its status, so
// tracing the body back through that local reaches the status parameter — and
// the status is a constant, not a payload (issue #485).
package main

import (
	"encoding/json"
	"net/http"
)

// Base mirrors the house context a project wraps its responses in.
type Base struct{ Resp http.ResponseWriter }

// HTTPError takes the status FIRST and the message variadically, and derives
// the message from the status when none is given. That derivation is what
// misleads the wrapper derivation: `v` traces back to `status`.
func (b *Base) HTTPError(status int, contents ...string) {
	v := http.StatusText(status)
	if len(contents) > 0 {
		v = contents[0]
	}
	http.Error(b.Resp, v, status)
}

// Item is the only thing this API actually serialises.
type Item struct {
	ID int `json:"id"`
}

func show(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	if r.URL.Query().Get("bad") != "" {
		b.HTTPError(http.StatusUnprocessableEntity, "unsupported mode")
		return
	}
	_ = json.NewEncoder(w).Encode(Item{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", show)
	_ = http.ListenAndServe(":8080", mux)
}
