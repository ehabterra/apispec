package webctx

import (
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

// Base writes every response. Handlers reach it through Context, which
// embeds it.
type Base struct {
	Resp ResponseWriter
	Req  *http.Request
}

// HTTPError answers the status in plain text through http.Error.
func (b *Base) HTTPError(status int, contents ...string) {
	v := http.StatusText(status)
	if len(contents) > 0 {
		v = contents[0]
	}
	http.Error(b.Resp, v, status)
}

// JSON encodes content on the same receiver field the plain-text writers use.
func (b *Base) JSON(status int, content any) {
	b.Resp.Header().Set("Content-Type", "application/json;charset=utf-8")
	b.Resp.WriteHeader(status)
	_ = json.NewEncoder(b.Resp).Encode(content)
}

// JSONVia is JSON through a local holding the receiver's writer: the
// destination is still the one the caller handed in.
func (b *Base) JSONVia(status int, content any) {
	dst := b.Resp
	dst.Header().Set("Content-Type", "application/json;charset=utf-8")
	dst.WriteHeader(status)
	_ = json.NewEncoder(dst).Encode(content)
}

// YAML encodes content as YAML on the response: a destination-gated encoder
// whose destination is the response field stays a responder.
func (b *Base) YAML(status int, content any) {
	b.Resp.Header().Set("Content-Type", "application/yaml")
	b.Resp.WriteHeader(status)
	_ = yaml.NewEncoder(b.Resp).Encode(content)
}

func (b *Base) plainTextInternal(skip, status int, bs []byte) {
	_ = skip
	b.Resp.Header().Set("Content-Type", "text/plain;charset=utf-8")
	b.Resp.Header().Set("X-Content-Type-Options", "nosniff")
	b.Resp.WriteHeader(status)
	_, _ = b.Resp.Write(bs)
}

func (b *Base) PlainTextBytes(status int, bs []byte) {
	b.plainTextInternal(2, status, bs)
}

func (b *Base) PlainText(status int, text string) {
	b.plainTextInternal(2, status, []byte(text))
}

func NewBase(w http.ResponseWriter, r *http.Request) *Base {
	return &Base{Resp: WrapResponseWriter(w), Req: r}
}
