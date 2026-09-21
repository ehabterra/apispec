// Package main is the header writes a handler can reach, split by whether the
// header belongs to THIS operation's response (issue #543).
//
// `w.Header().Set("Content-Type", …)` is how a handler states what it streams,
// and #517 reads it so a streamed body is documented at all. `req.Header.Set`
// on an *http.Request built to call somebody else is the same two calls on the
// same `http.Header` type — and says nothing about this operation's response.
// Neither does a header map that nothing sends.
//
// Documented (the header is the response's):
//
//	/csv          w.Header().Set, then a stream
//	/csv-var      h := w.Header(); h.Set
//	/csv-helper   a helper handed w sets it
//	/csv-wrapped  set through a wrapper built around w
//	/pdf-param   a helper handed w.Header() sets it
//	/csv-captured the response's header returned by a closure capturing w
//
// Not documented as a body (the header is somebody else's):
//
//	/callback     an OUTBOUND request's header, one call deeper; only a 303
//	/detached     a literal http.Header{} nothing sends; only a 204
//	/recorded     a recorder's header; only a 204
//	/forwarded    a helper handed the outbound request's header; only a 303
//	/proxied      the INCOMING request's header, rewritten before forwarding;
//	              only a 202
package main

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Item is what the ordinary route returns, so the fixture has a control.
type Item struct {
	ID string `json:"id"`
}

// exchange posts to somebody else's token endpoint. The Content-Type here is
// the OUTBOUND request's, one call deeper than the handler.
func exchange(code string) error {
	req, err := http.NewRequest(http.MethodPost, "https://example.test/token",
		strings.NewReader("code="+code))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// callback finishes an OAuth flow and redirects. It writes no body of its own.
func callback(w http.ResponseWriter, r *http.Request) {
	target := "/admin?result=ok"
	if err := exchange(r.URL.Query().Get("code")); err != nil {
		target = "/admin?result=error"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// detached sets a media type on a header map attached to nothing at all.
func detached(w http.ResponseWriter, r *http.Request) {
	h := http.Header{}
	h.Set("Content-Type", "application/pdf")
	_ = h
	w.WriteHeader(http.StatusNoContent)
}

// recorded sets a media type on a recorder, which is writer-shaped and is not
// the handler's response.
func recorded(w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusNoContent)
}

// declarePDF is a helper that declares a media type on whatever header it is
// handed — so whose header it is depends on the caller.
func declarePDF(h http.Header) {
	h.Set("Content-Type", "application/pdf")
}

// forwarded hands the OUTBOUND request's header to the helper.
func forwarded(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest(http.MethodPost, "https://example.test/hook", nil)
	if err == nil {
		declarePDF(req.Header)
		_, _ = http.DefaultClient.Do(req)
	}
	http.Redirect(w, r, "/done", http.StatusSeeOther)
}

func exportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	_ = csv.NewWriter(w).Write([]string{"id"})
}

func exportCSVVar(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/csv")
	_ = csv.NewWriter(w).Write([]string{"id"})
}

// declareCSV is handed the response writer itself.
func declareCSV(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/csv")
}

func exportCSVHelper(w http.ResponseWriter, r *http.Request) {
	declareCSV(w)
	_ = csv.NewWriter(w).Write([]string{"id"})
}

// loggingWriter wraps the response writer.
type loggingWriter struct {
	http.ResponseWriter
}

func exportCSVWrapped(w http.ResponseWriter, r *http.Request) {
	lw := &loggingWriter{w}
	lw.Header().Set("Content-Type", "text/csv")
	_ = csv.NewWriter(lw).Write([]string{"id"})
}

func exportPDFParam(w http.ResponseWriter, r *http.Request) {
	declarePDF(w.Header())
	_, _ = w.Write([]byte("id"))
}

// proxied rewrites the incoming request's media type before forwarding it —
// what a reverse proxy does. The header is the REQUEST's, not the response's.
func proxied(w http.ResponseWriter, r *http.Request) {
	r.Header.Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusAccepted)
}

// exportCSVCaptured reaches the response's header through a closure that
// captures w — a function call with no argument that nevertheless returns the
// response's own header.
func exportCSVCaptured(w http.ResponseWriter, r *http.Request) {
	hdr := func() http.Header { return w.Header() }
	h := hdr()
	h.Set("Content-Type", "text/csv")
	_ = csv.NewWriter(w).Write([]string{"id"})
}

// listItems is the control: an ordinary JSON response.
func listItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", callback)
	mux.HandleFunc("GET /detached", detached)
	mux.HandleFunc("GET /recorded", recorded)
	mux.HandleFunc("GET /forwarded", forwarded)
	mux.HandleFunc("GET /csv", exportCSV)
	mux.HandleFunc("GET /csv-var", exportCSVVar)
	mux.HandleFunc("GET /csv-helper", exportCSVHelper)
	mux.HandleFunc("GET /csv-wrapped", exportCSVWrapped)
	mux.HandleFunc("GET /pdf-param", exportPDFParam)
	mux.HandleFunc("GET /proxied", proxied)
	mux.HandleFunc("GET /csv-captured", exportCSVCaptured)
	mux.HandleFunc("GET /items", listItems)
	_ = http.ListenAndServe(":8080", mux)
}
