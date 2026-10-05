// Package main reads headers off the request and off outbound exchanges
// (issue #569).
//
// net/http.Header is the request's header map, and also the header map of
// every response an http.Client returns and every request a handler builds.
// A header read off one of those is not a parameter of the operation.
//
// Documented (the Header came from the request):
//
//	X-Request-Id     r.Header.Get
//	X-Trace          a helper handed r.Header
//	Accept-Language  r.Header.Values
//	X-Tenant         h := r.Header; h.Get
//
// Not documented (the Header came from somewhere else):
//
//	X-Rate-Remaining  resp.Header.Get on an http.Get response
//	Retry-After       a helper handed resp.Header
//	X-Upstream        resp.Header.Values on a client.Do response
//	X-Out             off an outbound request the handler built
//	X-Written         w.Header().Get — the server's own response header
//	X-Ctx-Upstream    off the response to a request built with the request's
//	                  context, inline and through a helper — the request is
//	                  handed r.Context(), so only the response's TYPE places it
//
// Known gap (change-detector, issue #574): X-Ctx-Out, read back off an
// outbound request built with r.Context(), is still documented. The request
// is handed the request's context, and nothing yet tells that apart from a
// helper handed the request's data.
package main

import (
	"encoding/json"
	"net/http"
)

var client = &http.Client{}

// backoff reads a key off whatever header map it is handed.
func backoff(h http.Header) string { return h.Get("Retry-After") }

// upstreamOf reads a key off whatever header map it is handed.
func upstreamOf(h http.Header) string { return h.Get("X-Ctx-Upstream") }

// traceOf is handed the request's headers.
func traceOf(h http.Header) string { return h.Get("X-Trace") }

func list(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Request-Id")
	trace := traceOf(r.Header)
	langs := r.Header.Values("Accept-Language")
	h := r.Header
	tenant := h.Get("X-Tenant")

	if resp, err := http.Get("https://upstream.example.test/v1"); err == nil {
		_ = resp.Header.Get("X-Rate-Remaining")
		_ = backoff(resp.Header)
		_ = resp.Body.Close()
	}

	out, _ := http.NewRequest(http.MethodGet, "https://upstream.example.test/v2", nil)
	out.Header.Set("X-Out", "1")
	_ = out.Header.Get("X-Out")
	if resp, err := client.Do(out); err == nil {
		_ = resp.Header.Values("X-Upstream")
		_ = resp.Body.Close()
	}

	ctxReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://upstream.example.test/v3", nil)
	_ = ctxReq.Header.Get("X-Ctx-Out")
	if resp, err := client.Do(ctxReq); err == nil {
		_ = resp.Header.Get("X-Ctx-Upstream")
		_ = upstreamOf(resp.Header)
		_ = resp.Body.Close()
	}

	w.Header().Set("X-Written", "1")
	_ = w.Header().Get("X-Written")
	_ = json.NewEncoder(w).Encode([]string{id, trace, tenant, langs[0]})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", list)
	_ = http.ListenAndServe(":8080", mux)
}
