package webctx

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ehabterra/apispec/testdata/embedded_context_responder/htmlrender"
)

var ErrNotExist = errors.New("not exist")

// Context embeds Base and adds HTML pages.
type Context struct {
	*Base
	Render htmlrender.Renderer
	Data   map[string]any
}

func (ctx *Context) HTML(status int, name string) {
	err := ctx.Render.HTML(ctx.Resp, status, name, ctx.Data)
	if err == nil {
		return
	}
	if name != "status/500" {
		ctx.ServerError("render failed", err)
		return
	}
	ctx.PlainText(http.StatusInternalServerError, "template system is broken")
}

func (ctx *Context) NotFound(logErr error) {
	ctx.notFoundInternal(logErr)
}

func (ctx *Context) notFoundInternal(logErr error) {
	_ = logErr
	showHTML := false
	for _, part := range ctx.Req.Header["Accept"] {
		if strings.Contains(part, "text/html") {
			showHTML = true
			break
		}
	}
	if !showHTML {
		ctx.plainTextInternal(3, http.StatusNotFound, []byte("Not found.\n"))
		return
	}
	ctx.HTML(http.StatusNotFound, "status/404")
}

func (ctx *Context) ServerError(logMsg string, logErr error) {
	if errors.Is(logErr, ErrNotExist) {
		ctx.notFoundInternal(logErr)
		return
	}
	ctx.serverErrorInternal(logMsg, logErr)
}

func (ctx *Context) serverErrorInternal(logMsg string, logErr error) {
	if logErr != nil {
		ctx.Data["ErrorMsg"] = logMsg + ": " + logErr.Error()
	}
	ctx.HTML(http.StatusInternalServerError, "status/500")
}

type contextKey struct{}

// Contexter builds the Context once per request and stores it in the request
// context; handlers read it back, so a handler's Context has no provenance a
// static reading can follow.
func Contexter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := &Context{Base: NewBase(w, r), Render: htmlrender.New(), Data: map[string]any{}}
		next.ServeHTTP(ctx.Resp, r.WithContext(context.WithValue(r.Context(), contextKey{}, ctx)))
	})
}

func FromRequest(r *http.Request) *Context {
	ctx, _ := r.Context().Value(contextKey{}).(*Context)
	return ctx
}
