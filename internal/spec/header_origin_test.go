// Copyright 2026 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func headerOriginFixture() (*metadata.Metadata, *responseDestResolver) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	// The real net/http preset: which types carry the request, which are the
	// writer, and what a Content-Type write is made on all come from it.
	return meta, newResponseDestResolver(DefaultHTTPConfig(), NewContextProvider(meta))
}

func hoIdent(meta *metadata.Metadata, name, typ string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	a.SetType(typ)
	return a
}

// Each way a header write's map is placed, and the one verdict that drops it:
// provably built where the handler's writer never reached (issue #543).
func TestHeaderWriteDetached(t *testing.T) {
	meta, r := headerOriginFixture()

	// The response writer itself, reached as the chain root of w.Header().Set.
	header := &metadata.CallGraphEdge{Receiver: hoIdent(meta, "w", "net/http.ResponseWriter")}
	set := &metadata.CallGraphEdge{ChainParent: header}
	if r.HeaderWriteDetached(&fakeNode{edge: set}) {
		t.Error("w.Header().Set was dropped — the response's own header must be read")
	}

	// A literal header map nothing sends.
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	if !r.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: lit}}) {
		t.Error("a Set on http.Header{} was kept")
	}

	// A FIELD of a request built by a function call — the outbound request.
	newReq := metadata.NewCallArgument(meta)
	newReq.SetKind(metadata.KindCall)
	newReq.Fun = metadata.NewCallArgument(meta)
	newReq.Fun.SetKind(metadata.KindSelector)
	newReq.Fun.X = hoIdent(meta, "http", "")
	newReq.Fun.Sel = hoIdent(meta, "NewRequest", "")
	req := hoIdent(meta, "req", "*net/http.Request")
	field := metadata.NewCallArgument(meta)
	field.SetKind(metadata.KindSelector)
	field.X, field.Sel = req, hoIdent(meta, "Header", "")
	outbound := &metadata.CallGraphEdge{
		Receiver: field,
		AssignmentMap: map[string][]metadata.Assignment{
			"req": {{Value: *newReq}},
		},
	}
	if !r.HeaderWriteDetached(&fakeNode{edge: outbound}) {
		t.Error("req.Header.Set on a request from http.NewRequest was kept")
	}

	// A METHOD result belongs to its receiver: h := w.Header() is the response.
	method := metadata.NewCallArgument(meta)
	method.SetKind(metadata.KindCall)
	method.Fun = metadata.NewCallArgument(meta)
	method.Fun.SetKind(metadata.KindSelector)
	method.Fun.X = hoIdent(meta, "w", "net/http.ResponseWriter")
	method.Fun.Sel = hoIdent(meta, "Header", "")
	method.Fun.ReceiverType = hoIdent(meta, "ResponseWriter", "ResponseWriter")
	viaVar := &metadata.CallGraphEdge{
		Receiver:      hoIdent(meta, "h", "net/http.Header"),
		AssignmentMap: map[string][]metadata.Assignment{"h": {{Value: *method}}},
	}
	if r.HeaderWriteDetached(&fakeNode{edge: viaVar}) {
		t.Error("h := w.Header(); h.Set was dropped")
	}

	// A context that merely exposes the writer (gin's c.Header) cannot be
	// placed, and unplaceable is kept.
	ctx := &metadata.CallGraphEdge{Receiver: hoIdent(meta, "c", "*github.com/gin-gonic/gin.Context")}
	if r.HeaderWriteDetached(&fakeNode{edge: ctx}) {
		t.Error("a framework context's own header write was dropped")
	}
}

// Every "cannot tell" is kept, and none of them may hang.
func TestHeaderWriteDetachedDeclines(t *testing.T) {
	meta, r := headerOriginFixture()

	if (&responseDestResolver{contextProvider: NewContextProvider(meta)}).HeaderWriteDetached(
		&fakeNode{edge: &metadata.CallGraphEdge{}}) {
		t.Error("a resolver with no writer types dropped a write — the gate must be off")
	}
	var nilResolver *responseDestResolver
	if nilResolver.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{}}) {
		t.Error("a nil resolver dropped a write")
	}
	if r.HeaderWriteDetached(nil) || r.HeaderWriteDetached(&fakeNode{}) {
		t.Error("no node, or no edge, dropped a write")
	}
	if r.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{}}) {
		t.Error("a call with no recorded receiver dropped a write")
	}

	// A cyclic chain ends at the hop bound as unknown rather than looping.
	a := &metadata.CallGraphEdge{}
	b := &metadata.CallGraphEdge{ChainParent: a}
	a.ChainParent = b
	if r.HeaderWriteDetached(&fakeNode{edge: a}) {
		t.Error("a cyclic chain dropped a write")
	}

	// Unknown value kinds, and a receiver wrapped in address-of, are placed by
	// what they wrap or left unknown.
	raw := metadata.NewCallArgument(meta)
	raw.SetKind(metadata.KindRaw)
	if r.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: raw}}) {
		t.Error("an unplaceable receiver dropped a write")
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	amp := metadata.NewCallArgument(meta)
	amp.SetKind(metadata.KindUnary)
	amp.X = lit
	if !r.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: amp}}) {
		t.Error("&http.Header{} was not placed through the address-of")
	}
	if got := r.valueOrigin(lit, nil, true, 0); got != headerOriginUnknown {
		t.Errorf("valueOrigin with no frame = %v, want unknown", got)
	}
	if got := r.valueOrigin(lit, &fakeNode{}, true, 0); got != headerOriginUnknown {
		t.Errorf("valueOrigin with no edge = %v, want unknown", got)
	}
}

// A header PARAMETER is its caller's argument: a helper handed a detached map
// is dropped, one handed the response's is kept.
func TestHeaderWriteDetachedFollowsParameter(t *testing.T) {
	meta, r := headerOriginFixture()

	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	caller := &fakeNode{edge: &metadata.CallGraphEdge{ParamArgMap: map[string]metadata.CallArgument{"h": *lit}}}
	inHelper := &fakeNode{parent: caller, edge: &metadata.CallGraphEdge{Receiver: hoIdent(meta, "h", "")}}
	if !r.HeaderWriteDetached(inHelper) {
		t.Error("a helper handed http.Header{} kept its write")
	}

	w := hoIdent(meta, "w", "net/http.ResponseWriter")
	fromW := &fakeNode{edge: &metadata.CallGraphEdge{ParamArgMap: map[string]metadata.CallArgument{"h": *w}}}
	inHelper = &fakeNode{parent: fromW, edge: &metadata.CallGraphEdge{Receiver: hoIdent(meta, "h", "")}}
	if r.HeaderWriteDetached(inHelper) {
		t.Error("a helper handed the response writer dropped its write")
	}
}

// A header map read as a FIELD is its owner's, and an owner that carries the
// request and is no writer makes it the request's header — `r.Header.Set` in a
// proxy rewriting what it forwards (review of #545).
func TestHeaderWriteDetachedRequestField(t *testing.T) {
	meta, r := headerOriginFixture()

	field := func(owner *metadata.CallArgument) *metadata.CallArgument {
		f := metadata.NewCallArgument(meta)
		f.SetKind(metadata.KindSelector)
		f.X, f.Sel = owner, hoIdent(meta, "Header", "")
		return f
	}
	incoming := &metadata.CallGraphEdge{Receiver: field(hoIdent(meta, "r", "*net/http.Request"))}
	if !r.HeaderWriteDetached(&fakeNode{edge: incoming}) {
		t.Error("r.Header.Set on the handler's own request was kept as a response")
	}

	// An owner that is not a request stays unplaced: the rule reads the owner's
	// TYPE, never the field's name.
	other := &metadata.CallGraphEdge{Receiver: field(hoIdent(meta, "cfg", "example.com/app.Config"))}
	if r.HeaderWriteDetached(&fakeNode{edge: other}) {
		t.Error("a Header field on a non-request owner was dropped")
	}

	// A selector that LEADS to the map is not the map: `c.Writer.Header()` has
	// the gin context as the owner of Writer, and the context carries the
	// request too. The chain puts the selector one position up.
	ginCtx := hoIdent(meta, "c", "*github.com/gin-gonic/gin.Context")
	gin := newResponseDestResolverForTest(meta, `^\*?github\.com/gin-gonic/gin\.Context$`)
	writerSel := metadata.NewCallArgument(meta)
	writerSel.SetKind(metadata.KindSelector)
	writerSel.X, writerSel.Sel = ginCtx, hoIdent(meta, "Writer", "")
	header := &metadata.CallGraphEdge{Receiver: writerSel}
	if gin.HeaderWriteDetached(&fakeNode{edge: &metadata.CallGraphEdge{ChainParent: header}}) {
		t.Error("c.Writer.Header().Set was dropped — the context's writer is the response")
	}
}

// newResponseDestResolverForTest is the net/http preset with one more request
// type, the way gin's preset lists its context beside *http.Request.
func newResponseDestResolverForTest(meta *metadata.Metadata, requestType string) *responseDestResolver {
	cfg := DefaultHTTPConfig()
	cfg.Framework.RequestContext.TypeRegexes = append(cfg.Framework.RequestContext.TypeRegexes, requestType)
	return newResponseDestResolver(cfg, NewContextProvider(meta))
}

// A plain function's result is placed by what it returns (review of #545).
func TestFunctionResultOrigin(t *testing.T) {
	meta, r := headerOriginFixture()
	call := func(fun, pkg, typ string) *metadata.CallArgument {
		c := metadata.NewCallArgument(meta)
		c.SetKind(metadata.KindCall)
		c.Fun = hoIdent(meta, fun, "")
		c.Fun.SetPkg(pkg)
		c.SetType(typ)
		return c
	}
	for _, tc := range []struct {
		name string
		call *metadata.CallArgument
		want headerOrigin
	}{
		{"make constructs in place", call("make", "", "net/http.Header"), headerOriginDetached},
		{"new constructs in place", call("new", "", "*net/http.Header"), headerOriginDetached},
		// A header map can be anyone's — a closure capturing w returns the
		// response's own with no argument to show it.
		{"returns a header map", call("hdr", "example.com/app", "net/http.Header"), headerOriginUnknown},
		{"returns the writer", call("current", "example.com/app", "net/http.ResponseWriter"), headerOriginResponse},
		{"returns a writer-compatible interface", call("sink", "example.com/app", "io.Writer"), headerOriginUnknown},
		{"type unrecorded (a tuple)", call("pair", "example.com/app", ""), headerOriginUnknown},
		{"builds a recorder", call("NewRecorder", "net/http/httptest", "*net/http/httptest.ResponseRecorder"), headerOriginDetached},
		// A same-named function in a package is not the builtin.
		{"a package's own make", call("make", "example.com/app", "net/http.Header"), headerOriginUnknown},
	} {
		if got := r.functionResultOrigin(tc.call); got != tc.want {
			t.Errorf("%s: functionResultOrigin = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A METHOD whose result is itself the writer answers outright, rather than
	// by its receiver: `resp := c.Response()` on echo.
	method := metadata.NewCallArgument(meta)
	method.SetKind(metadata.KindCall)
	method.SetType("net/http.ResponseWriter")
	method.Fun = metadata.NewCallArgument(meta)
	method.Fun.SetKind(metadata.KindSelector)
	method.Fun.X = hoIdent(meta, "c", "example.com/fw.Context")
	method.Fun.Sel = hoIdent(meta, "Response", "")
	method.Fun.ReceiverType = hoIdent(meta, "Context", "Context")
	if got := r.valueOrigin(method, &fakeNode{edge: &metadata.CallGraphEdge{}}, false, 0); got != headerOriginResponse {
		t.Errorf("a method returning the writer = %v, want response", got)
	}
}
