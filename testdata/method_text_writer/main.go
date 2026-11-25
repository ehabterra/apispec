// A response writer declared as a METHOD that writes through a receiver field
// must document exactly what the same body written by a plain FUNCTION
// documents (issue #645).
//
// A method that forwards its own parameters into a response write is derived
// as the project's responder, and the derived pattern documents the body at
// every call site. It used to lift the body without the media type the write
// gives it, so every such responder also answered application/json beside the
// text/plain the method declares. A function is never derived, so the same
// code written as one documented text/plain only.
package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Base struct {
	Resp http.ResponseWriter
}

// PlainText declares text/plain and writes the bytes it is handed.
func (b *Base) PlainText(status int, text string) {
	b.Resp.Header().Set("Content-Type", "text/plain;charset=utf-8")
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write([]byte(text))
}

// Bytes declares nothing: the server sniffs what it writes.
func (b *Base) Bytes(status int, data []byte) {
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write(data)
}

// Fail answers through http.Error, which states text/plain itself.
func (b *Base) Fail(status int, msg string) {
	http.Error(b.Resp, msg, status)
}

// MarshalJSON writes a serializer's document, not raw bytes: the body is the
// value it was handed, typed at each call site.
func (b *Base) MarshalJSON(status int, v any) {
	bs, _ := json.Marshal(v)
	b.Resp.Header().Set("Content-Type", "application/json")
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write(bs)
}

// Blob declares JSON and writes bytes its CALLER serialized: whether they are
// a document is a fact about each call site, not about the method.
func (b *Base) Blob(status int, bs []byte) {
	b.Resp.Header().Set("Content-Type", "application/json")
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write(bs)
}

// Fail500 wraps a wrapper: derived from PlainText's derived pattern.
func (b *Base) Fail500(text string) {
	b.PlainText(http.StatusInternalServerError, text)
}

// The same bodies, written by functions.

func plainText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain;charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}

func writeBytes(w http.ResponseWriter, status int, data []byte) {
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func blob(w http.ResponseWriter, status int, bs []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(bs)
}

func fail(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}

var errMissing = errors.New("missing")

func load(id string) (*Item, error) {
	if id == "" {
		return nil, errMissing
	}
	return &Item{ID: id}, nil
}

func methodText(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	if _, err := load(r.URL.Query().Get("id")); err != nil {
		b.PlainText(http.StatusInternalServerError, "failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func funcText(w http.ResponseWriter, r *http.Request) {
	if _, err := load(r.URL.Query().Get("id")); err != nil {
		plainText(w, http.StatusInternalServerError, "failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func methodBytes(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	b.Bytes(http.StatusBadRequest, []byte(r.URL.Query().Get("id")))
}

func funcBytes(w http.ResponseWriter, r *http.Request) {
	writeBytes(w, http.StatusBadRequest, []byte(r.URL.Query().Get("id")))
}

func methodFail(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	b.Fail(http.StatusConflict, "conflict")
}

func funcFail(w http.ResponseWriter, r *http.Request) {
	fail(w, http.StatusConflict, "conflict")
}

func methodJSON(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	item, err := load(r.URL.Query().Get("id"))
	if err != nil {
		b.PlainText(http.StatusNotFound, "not found")
		return
	}
	b.MarshalJSON(http.StatusOK, item)
}

func methodBlob(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	bs, _ := json.Marshal(Item{ID: "1"})
	b.Blob(http.StatusCreated, bs)
}

func funcBlob(w http.ResponseWriter, r *http.Request) {
	bs, _ := json.Marshal(Item{ID: "1"})
	blob(w, http.StatusCreated, bs)
}

func methodFail500(w http.ResponseWriter, r *http.Request) {
	b := &Base{Resp: w}
	b.Fail500("boom")
}

func main() {
	r := chi.NewRouter()
	r.Get("/method/text", methodText)
	r.Get("/func/text", funcText)
	r.Get("/method/bytes", methodBytes)
	r.Get("/func/bytes", funcBytes)
	r.Get("/method/fail", methodFail)
	r.Get("/func/fail", funcFail)
	r.Get("/method/json", methodJSON)
	r.Get("/method/blob", methodBlob)
	r.Get("/func/blob", funcBlob)
	r.Get("/method/fail500", methodFail500)
	_ = http.ListenAndServe(":8080", r)
}
