// Package main reads query keys off url.Values from the request and from
// elsewhere (issue #552).
//
// url.Values is the request's query, and also the query of any URL a handler
// parses, builds or forwards. A query key read off one the client never sent
// is not a parameter of the operation.
//
// Documented (the Values came from the request):
//
//	page     r.URL.Query().Get
//	sort     q := r.URL.Query(); q.Get
//	limit    a helper handed r.URL.Query()
//	filter   url.ParseQuery(r.URL.RawQuery) — parsed, but from the request
//	cursor   a helper handed r.URL itself
//	mq       a method on a literal, handed the request
//
// Not documented (the Values were built elsewhere):
//
//	clientname   off a connection URI in a package-level variable
//	region       off a URL literal
//	token        off a url.Values literal built for an outbound request
//	skipverify   a helper handed a URL a method built from a connection
//	             string it was handed — the shape of gitea's Redis client
//	outq         off an OUTBOUND request's URL, built in the handler
//	sig          off an outbound request a helper is handed — a
//	             *http.Request, but not this operation's
package main

import (
	"encoding/json"
	"net/http"
	"net/url"
)

var connString = "redis://localhost:6379/0?clientname=api"

// connect reads a key off a CONFIG URI.
func connect() string {
	u, err := url.Parse(connString)
	if err != nil {
		return ""
	}
	return u.Query().Get("clientname")
}

// endpoint reads a key off a URL the handler writes itself.
func endpoint() string {
	u, _ := url.Parse("https://api.example.test/v1?region=eu")
	return u.Query().Get("region")
}

// outbound builds form values for a call to somebody else.
func outbound() string {
	form := url.Values{"token": {"abc"}}
	return form.Get("token")
}

// toURI builds a URL from a string: a new value, not the request's.
func toURI(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

// tlsOf reads a key off whatever URL it is handed.
func tlsOf(uri *url.URL) string { return uri.Query().Get("skipverify") }

type manager struct{}

// client is handed a connection string and builds its URL from it.
func (manager) client(connection string) string {
	uri := toURI(connection)
	return tlsOf(uri)
}

// cursorOf is handed the request's URL.
func cursorOf(u *url.URL) string { return u.Query().Get("cursor") }

// reader is a value whose method is handed the request.
type reader struct{}

func (reader) values(r *http.Request) url.Values { return r.URL.Query() }

// sign reads a key off whatever request it is handed.
func sign(req *http.Request) string { return req.URL.Query().Get("sig") }

// limitOf is handed the request's query.
func limitOf(q url.Values) string { return q.Get("limit") }

func list(w http.ResponseWriter, r *http.Request) {
	_, _, _ = connect(), endpoint(), outbound()
	_ = manager{}.client(connString)
	cursor := cursorOf(r.URL)
	mq := reader{}.values(r).Get("mq")

	out, _ := http.NewRequest(http.MethodGet, "https://api.example.test/v1?outq=1&sig=2", nil)
	_ = out.URL.Query().Get("outq")
	_ = sign(out)

	page := r.URL.Query().Get("page")
	q := r.URL.Query()
	sort := q.Get("sort")
	limit := limitOf(r.URL.Query())
	vals, _ := url.ParseQuery(r.URL.RawQuery)
	filter := vals.Get("filter")

	_ = json.NewEncoder(w).Encode([]string{page, sort, limit, filter, cursor, mq})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", list)
	_ = http.ListenAndServe(":8080", mux)
}
