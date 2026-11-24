// A configured parameter accessor handed a value produced in a shared helper
// (issue #615): the parameter it reads belongs to the routes that call it,
// never to every route that merely reaches the helper producing the value.
//
// The test configures readFlag as a query accessor (name at argument 1).
// `readFlag(values, "verbose", &verbose)` binds getItem's `values` to
// readFlag's parameter, and `values` traces to the call that produced it
// inside requestValues. The binding argument hangs under that producer, so
// listItems — which calls requestValues but never readFlag — reaches it too.
// That argument node carries the readFlag call's edge, and read as that call
// it documented `verbose` on listItems.
//
// readFlag's body calls a method on `values` — that call is what the frame
// carries — but deliberately one that reads no parameter (Encode, not Get), so
// the accessor call is the only source of `verbose`.
package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// requestValues is shared by both routes; the values it returns are produced
// here, by a call outside the module, so tracing the caller's `values` ends at
// this assignment.
func requestValues(r *http.Request) url.Values {
	values, _ := url.ParseQuery(r.URL.RawQuery)
	return values
}

// readFlag reports whether the named flag is present. It reads its parameter,
// which is what binds the caller's `values` to it.
func readFlag(values url.Values, name string, dst *bool) {
	*dst = strings.Contains(values.Encode(), name+"=")
}

// listItems reaches requestValues too but never reads the flag.
func listItems(w http.ResponseWriter, r *http.Request) {
	values := requestValues(r)
	_, _ = w.Write([]byte(values.Encode()))
}

// getItem reads the flag: `verbose` is its query parameter.
func getItem(w http.ResponseWriter, r *http.Request) {
	values := requestValues(r)
	var verbose bool
	readFlag(values, "verbose", &verbose)
	if verbose {
		_, _ = w.Write([]byte(r.PathValue("id") + " (verbose)"))
		return
	}
	_, _ = w.Write([]byte(r.PathValue("id")))
}

// ctxKey keys the request stored in a context.
type ctxKey struct{}

// requestScheme reads the scheme a proxy forwarded. The header read is an
// argument of another call, and still a statement of the route that calls it.
func requestScheme(req *http.Request) string {
	if proto, ok := parseProto(req.Header.Get("X-Forwarded-Proto")); ok {
		return proto
	}
	return "http"
}

func parseProto(v string) (string, bool) {
	return v, v == "http" || v == "https"
}

// currentURL takes the request back out of a context, so `req` is produced by
// a call outside the module and requestScheme's reads on it hang under that
// producer — inside the binding frame of requestScheme(req).
func currentURL(ctx context.Context) string {
	req, ok := ctx.Value(ctxKey{}).(*http.Request)
	if !ok {
		return "/"
	}
	return requestScheme(req) + "://" + req.Host
}

// getSelf reads the forwarded header through currentURL: X-Forwarded-Proto is
// its header parameter.
func getSelf(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), ctxKey{}, r)
	_, _ = w.Write([]byte(currentURL(ctx)))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", listItems)
	mux.HandleFunc("GET /items/{id}", getItem)
	mux.HandleFunc("GET /self", getSelf)
	_ = http.ListenAndServe(":8080", mux)
}
