// A request context embeds the type that writes responses, and that type writes
// through the project's own ResponseWriter interface, implemented by a
// decorator (issue #645). The writes the decorator makes are reached through
// interface dispatch, so they run as part of the call that dispatched them:
// a plain-text helper's bytes go with its own Content-Type and status, and an
// HTML renderer's declaration with the status it writes after it. Attributed
// to the handler instead, they claimed whatever status the handler wrote last,
// and `ctx.HTTPError(409, …)` documented application/json and text/html beside
// its text/plain.
//
// The renderer package sorts before the handler and the writer package after
// it, as in the project that showed it: pairing reads fragments in file order.
//
// Dispatch is the negative for the YAML half: a method that encodes into its
// own buffer is no responder, and a handler that calls it documents no YAML.
// Export is the positive: an encoder on the response field stays one.
package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ehabterra/apispec/testdata/embedded_context_responder/webctx"
)

type Options struct {
	Challenge string `json:"challenge"`
}

var errMissing = errors.New("missing")

func lookup(name string) (*Options, error) {
	if name == "" {
		return nil, errMissing
	}
	return &Options{Challenge: name}, nil
}

func wrap(h func(*webctx.Context)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(webctx.FromRequest(r))
	}
}

// Register answers JSON on success and plain-text errors otherwise.
func Register(ctx *webctx.Context) {
	if ctx.Req.URL.Query().Get("off") != "" {
		ctx.HTTPError(http.StatusNotFound)
		return
	}
	opts, err := lookup(ctx.Req.URL.Query().Get("name"))
	if err != nil {
		ctx.ServerError("lookup", err)
		return
	}
	if opts.Challenge == "taken" {
		ctx.HTTPError(http.StatusConflict, "Name already taken")
		return
	}
	ctx.JSON(http.StatusOK, opts)
}

// Plain answers plain text through PlainText.
func Plain(ctx *webctx.Context) {
	if _, err := lookup(""); err != nil {
		ctx.PlainText(http.StatusBadRequest, "bad name")
		return
	}
	ctx.PlainTextBytes(http.StatusOK, []byte("ok"))
}

// Missing answers 404 through NotFound (plain text or the 404 page).
func Missing(ctx *webctx.Context) {
	ctx.NotFound(nil)
}

// Page renders an HTML page.
func Page(ctx *webctx.Context) {
	ctx.HTML(http.StatusOK, "page")
}

// Dispatch encodes a job into a buffer, then renders a page: the YAML never
// reaches the response.
func Dispatch(ctx *webctx.Context) {
	wf := &webctx.Workflow{}
	if err := wf.SetJob("build", &webctx.Job{Name: ctx.Req.URL.Query().Get("job")}); err != nil {
		ctx.ServerError("SetJob", err)
		return
	}
	ctx.HTML(http.StatusOK, "dispatch")
}

// Export answers YAML through the context.
func Export(ctx *webctx.Context) {
	ctx.YAML(http.StatusOK, &Options{Challenge: "x"})
}

// Override sets Content-Type twice before its status: only the second is in
// effect when WriteHeader commits the headers, so the 400 is text/plain only.
func Override(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("bad"))
}

// Accepted overrides its declaration and sends no body: the 202 is text/csv.
func Accepted(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Type", "text/csv")
	w.WriteHeader(http.StatusAccepted)
}

// Branched overrides only one arm: the html declaration is still in effect on
// the other, so the 202 keeps it.
func Branched(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if r.URL.Query().Get("csv") != "" {
		w.Header().Set("Content-Type", "text/csv")
	}
	w.WriteHeader(http.StatusAccepted)
}

func main() {
	r := chi.NewRouter()
	r.Use(webctx.Contexter)
	r.Post("/register", wrap(Register))
	r.Get("/plain", wrap(Plain))
	r.Get("/missing", wrap(Missing))
	r.Get("/page", wrap(Page))
	r.Get("/dispatch", wrap(Dispatch))
	r.Get("/export", wrap(Export))
	r.Get("/override", Override)
	r.Get("/accepted", Accepted)
	r.Get("/branched", Branched)
	_ = http.ListenAndServe(":8080", r)
}
