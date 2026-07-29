// Fixture: a response header read reached through an INTERFACE that the project
// supplies its own implementation of.
//
// `net/http`'s param pattern documents `Header().Get(k)` as a request header and
// excludes the case where the header belongs to the server's own response
// (ExcludeRecvOriginRegex, `responseWriterOriginRegex`). That exclusion names the
// INTERFACE — `net/http.ResponseWriter` — because that is what the source says.
//
// A resolved (SSA+VTA) call graph replaces the interface with the concrete type
// that actually runs, which is the whole point of resolving. Here the concrete
// type is the project's own `*recorder`, which the exclusion has never heard of,
// so the response header is documented as a request parameter.
//
// The shape is taken from a real render helper: middleware wraps the writer in
// the project's own type, and the renderer takes an io.Writer, asserts it back
// to http.ResponseWriter, and defaults the Content-Type if it is unset.
package main

import (
	"encoding/json"
	"io"
	"net/http"
)

type Greeting struct {
	Text string `json:"text"`
}

// recorder is the project's own http.ResponseWriter — the shape every access-log
// or status-capturing middleware has.
type recorder struct {
	http.ResponseWriter
	status int
}

func (rec *recorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func withRecorder(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(&recorder{ResponseWriter: w}, r)
	}
}

// render defaults the RESPONSE's own Content-Type before writing. Every Header
// call here is on the response, none of them is a request parameter.
func render(w io.Writer, v any) {
	if rw, ok := w.(http.ResponseWriter); ok {
		if rw.Header().Get("Content-Type") == "" {
			rw.Header().Set("Content-Type", "application/json")
		}
	}
	_ = json.NewEncoder(w).Encode(v)
}

func greet(w http.ResponseWriter, r *http.Request) {
	render(w, Greeting{Text: "hi"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /greet", withRecorder(greet))
	_ = http.ListenAndServe(":8080", mux)
}
