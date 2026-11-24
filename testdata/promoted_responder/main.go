// A request context that embeds a responder type promotes the responder's
// methods: `ctx.PlainText(…)` is written against Context but runs Base's
// (issue #643). Every status and body below is produced inside Base, so it is
// only documented when the promoted call reaches Base's body.
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

// Base writes the response. Every handler reaches it through an embedding
// type, never by name.
type Base struct {
	Resp http.ResponseWriter
	Req  *http.Request
}

func (b *Base) Status(status int) {
	b.Resp.WriteHeader(status)
}

func (b *Base) plainTextInternal(status int, bs []byte) {
	b.Resp.Header().Set("Content-Type", "text/plain;charset=utf-8")
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write(bs)
}

func (b *Base) PlainText(status int, text string) {
	b.plainTextInternal(status, []byte(text))
}

func (b *Base) JSON(status int, v any) {
	b.Resp.Header().Set("Content-Type", "application/json")
	b.Resp.WriteHeader(status)
	_ = json.NewEncoder(b.Resp).Encode(v)
}

// Context embeds Base: Status, PlainText and JSON are Base's, promoted.
type Context struct {
	*Base
}

// notFound calls an UNEXPORTED promoted method.
func (ctx *Context) notFound() {
	ctx.plainTextInternal(http.StatusNotFound, []byte("not found"))
}

// PrivateContext embeds Context, so Base's methods are promoted through two
// fields.
type PrivateContext struct {
	*Context
}

// Shadow declares its own Status, which shadows Base's and writes nothing. A
// call to it is Shadow's, so its argument is not a response status.
type Shadow struct {
	*Base
}

func (s *Shadow) Status(code int) {
	_ = code
}

func newBase(w http.ResponseWriter, r *http.Request) *Base {
	return &Base{Resp: w, Req: r}
}

func wrap(h func(*Context)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(&Context{Base: newBase(w, r)})
	}
}

func wrapPrivate(h func(*PrivateContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(&PrivateContext{Context: &Context{Base: newBase(w, r)}})
	}
}

func wrapShadow(h func(*Shadow)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(&Shadow{Base: newBase(w, r)})
	}
}

var errMissing = errors.New("missing")

func load(id string) (*Item, error) {
	if id == "" {
		return nil, errMissing
	}
	return &Item{ID: id}, nil
}

// Unpin answers 204 with no body, or 500 in plain text.
func Unpin(ctx *Context) {
	if _, err := load(ctx.Req.URL.Query().Get("id")); err != nil {
		ctx.PlainText(http.StatusInternalServerError, "unpin failed")
		return
	}
	ctx.Status(http.StatusNoContent)
}

// UnpinExplicit is Unpin written through the embedded field by name. The
// promoted call and this one run the same code and must document the same
// responses.
func UnpinExplicit(ctx *Context) {
	if _, err := load(ctx.Req.URL.Query().Get("id")); err != nil {
		ctx.Base.PlainText(http.StatusInternalServerError, "unpin failed")
		return
	}
	ctx.Base.Status(http.StatusNoContent)
}

// GetItem answers the item, or 404 through an unexported promoted method.
func GetItem(ctx *Context) {
	item, err := load(ctx.Req.URL.Query().Get("id"))
	if err != nil {
		ctx.notFound()
		return
	}
	ctx.JSON(http.StatusOK, item)
}

// Internal reaches Base through two embedded fields.
func Internal(ctx *PrivateContext) {
	if _, err := load(""); err != nil {
		ctx.PlainText(http.StatusServiceUnavailable, "busy")
		return
	}
	ctx.Status(http.StatusAccepted)
}

// Teapot calls Shadow's own Status, which writes nothing: 418 must not be
// documented.
func Teapot(s *Shadow) {
	s.Status(http.StatusTeapot)
	s.JSON(http.StatusCreated, Item{})
}

func main() {
	r := chi.NewRouter()
	r.Delete("/unpin", wrap(Unpin))
	r.Delete("/unpin-explicit", wrap(UnpinExplicit))
	r.Get("/item", wrap(GetItem))
	r.Post("/internal", wrapPrivate(Internal))
	r.Post("/teapot", wrapShadow(Teapot))
	_ = http.ListenAndServe(":8080", r)
}
