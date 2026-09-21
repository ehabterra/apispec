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

func roIdent(meta *metadata.Metadata, name, typ, pkg string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	a.SetType(typ)
	a.SetPkg(pkg)
	return a
}

func roMatcher(meta *metadata.Metadata) *ParamPatternMatcherImpl {
	return NewParamPatternMatcher(ParamPattern{RequireRequestOrigin: true}, DefaultHTTPConfig(), NewContextProvider(meta))
}

// Each way a query read's receiver is placed (issue #552).
func TestReadsOutsideRequest(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool(), Packages: map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{"app.go": {Variables: map[string]*metadata.Variable{"connString": {}}}}},
	}}
	p := roMatcher(meta)

	// r.URL.Query().Get: the chain root's receiver is r.URL, whose owner is
	// the request.
	rURL := metadata.NewCallArgument(meta)
	rURL.SetKind(metadata.KindSelector)
	rURL.X, rURL.Sel = roIdent(meta, "r", "*net/http.Request", "app"), roIdent(meta, "URL", "", "")
	query := &metadata.CallGraphEdge{Receiver: rURL}
	if p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{ChainParent: query}}) {
		t.Error("r.URL.Query().Get was dropped")
	}

	// A package-level variable: configuration, never the request.
	if !p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: roIdent(meta, "connString", "string", "app")}}) {
		t.Error("a read off a package-level variable was kept")
	}

	// A url.Values literal, and a literal argument to a plain function.
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	if !p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: lit}}) {
		t.Error("a read off a url.Values literal was kept")
	}
	strLit := metadata.NewCallArgument(meta)
	strLit.SetKind(metadata.KindLiteral)
	strLit.SetValue(`"https://x?y=1"`)
	parse := metadata.NewCallArgument(meta)
	parse.SetKind(metadata.KindCall)
	parse.Fun = roIdent(meta, "Parse", "", "net/url")
	parse.Args = []*metadata.CallArgument{strLit}
	parsed := &metadata.CallGraphEdge{
		Receiver:      roIdent(meta, "u", "*net/url.URL", "app"),
		AssignmentMap: map[string][]metadata.Assignment{"u": {{Value: *parse}}},
	}
	if !p.readsOutsideRequest(&fakeNode{edge: parsed}) {
		t.Error("a read off url.Parse(literal) was kept")
	}

	// A plain function handed something from the request stays the request's:
	// url.ParseQuery(r.URL.RawQuery).
	raw := metadata.NewCallArgument(meta)
	raw.SetKind(metadata.KindSelector)
	raw.X, raw.Sel = rURL, roIdent(meta, "RawQuery", "string", "")
	parseQuery := metadata.NewCallArgument(meta)
	parseQuery.SetKind(metadata.KindCall)
	parseQuery.Fun = roIdent(meta, "ParseQuery", "", "net/url")
	parseQuery.Args = []*metadata.CallArgument{raw}
	fromRequest := &metadata.CallGraphEdge{
		Receiver:      roIdent(meta, "vals", "net/url.Values", "app"),
		AssignmentMap: map[string][]metadata.Assignment{"vals": {{Value: *parseQuery}}},
	}
	if p.readsOutsideRequest(&fakeNode{edge: fromRequest}) {
		t.Error("url.ParseQuery(r.URL.RawQuery) was dropped")
	}

	// A plain function none of whose arguments reach the request builds a
	// value that is not the request's — even when an argument cannot be
	// placed, which is how a connection string arrives through configuration
	// parameters (the write side's constructor rule).
	unplaced := metadata.NewCallArgument(meta)
	unplaced.SetKind(metadata.KindCall)
	unplaced.Fun = roIdent(meta, "ToRedisURI", "", "app")
	unplaced.Args = []*metadata.CallArgument{roIdent(meta, "connection", "string", "app")}
	constructed := &metadata.CallGraphEdge{
		Receiver:      roIdent(meta, "u", "*net/url.URL", "app"),
		AssignmentMap: map[string][]metadata.Assignment{"u": {{Value: *unplaced}}},
	}
	if !p.readsOutsideRequest(&fakeNode{edge: constructed}) {
		t.Error("a URL a plain function built from an unplaceable string was kept")
	}

	// A METHOD on something that cannot be placed stays unknown and is kept.
	method := metadata.NewCallArgument(meta)
	method.SetKind(metadata.KindCall)
	method.Fun = metadata.NewCallArgument(meta)
	method.Fun.SetKind(metadata.KindSelector)
	method.Fun.X = roIdent(meta, "holder", "*app.Holder", "app")
	method.Fun.Sel = roIdent(meta, "Values", "", "")
	method.Fun.ReceiverType = roIdent(meta, "Holder", "Holder", "app")
	viaMethod := &metadata.CallGraphEdge{
		Receiver:      roIdent(meta, "v", "net/url.Values", "app"),
		AssignmentMap: map[string][]metadata.Assignment{"v": {{Value: *method}}},
	}
	if p.readsOutsideRequest(&fakeNode{edge: viaMethod}) {
		t.Error("a method on an unplaceable receiver dropped the read")
	}

	// A qualified package-level variable: setting.Conn.
	qual := metadata.NewCallArgument(meta)
	qual.SetKind(metadata.KindSelector)
	qual.X, qual.Sel = roIdent(meta, "setting", "", "app"), roIdent(meta, "connString", "", "")
	qual.SetPkg("app")
	if !p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: qual}}) {
		t.Error("a read off a qualified package-level variable was kept")
	}
}

// Every "cannot tell" keeps the parameter.
func TestReadsOutsideRequestDeclines(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	p := roMatcher(meta)
	var nilMatcher *ParamPatternMatcherImpl
	if nilMatcher.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{}}) ||
		p.readsOutsideRequest(nil) || p.readsOutsideRequest(&fakeNode{}) {
		t.Error("a missing matcher, node or edge dropped a read")
	}
	if p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{}}) {
		t.Error("a call with no recorded receiver dropped a read")
	}
	noTypes := NewParamPatternMatcher(ParamPattern{RequireRequestOrigin: true}, &APISpecConfig{}, NewContextProvider(meta))
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	if noTypes.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: lit}}) {
		t.Error("with no request types configured there is nothing to prove against, yet a read was dropped")
	}
	a := &metadata.CallGraphEdge{}
	b := &metadata.CallGraphEdge{ChainParent: a}
	a.ChainParent = b
	if p.readsOutsideRequest(&fakeNode{edge: a}) {
		t.Error("a cyclic chain dropped a read")
	}
	raw := metadata.NewCallArgument(meta)
	raw.SetKind(metadata.KindRaw)
	if p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{Receiver: raw}}) {
		t.Error("an unplaceable receiver dropped a read")
	}
	w := requestOriginWalker{cp: NewContextProvider(meta)}
	if w.valueOrigin(raw, nil, 0) != requestOriginUnknown || w.valueOrigin(raw, &fakeNode{}, 0) != requestOriginUnknown {
		t.Error("no frame must be unknown")
	}
	if w.isPackageVar("", "x") || w.isPackageVar("missing", "x") {
		t.Error("an unknown package declares nothing")
	}
}

// A parameter the tree path does not bind is placed by what every caller
// passes for it (issue #552): any caller handing it the request makes it the
// request's; every caller handing it something built elsewhere makes it
// elsewhere; anything else is no answer.
func TestParamOriginFromCallers(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	pool := meta.StringPool
	call := func(name, pos string) metadata.Call {
		return metadata.Call{Meta: meta, Name: pool.Get(name), Pkg: pool.Get("app"), Position: pool.Get(pos), RecvType: -1, Scope: -1, SignatureStr: -1}
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	req := roIdent(meta, "r", "*net/http.Request", "app")

	build := func(args ...metadata.CallArgument) (*metadata.CallGraphEdge, requestOriginWalker) {
		meta.CallGraph = nil
		for i, a := range args {
			meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
				Caller: call("caller", string(rune('1'+i))), Callee: call("helper", "9"),
				ParamArgMap: map[string]metadata.CallArgument{"u": a},
			})
		}
		meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{Caller: call("helper", "9"), Callee: call("Get", "8")})
		meta.BuildCallGraphMaps()
		return &meta.CallGraph[len(meta.CallGraph)-1], requestOriginWalker{cp: NewContextProvider(meta), requestTypes: compileAll(DefaultHTTPConfig().Framework.RequestContext.TypeRegexes)}
	}

	if edge, w := build(*lit, *lit); true {
		if got, ok := w.paramOriginFromCallers("u", edge, 0); !ok || got != requestOriginElsewhere {
			t.Errorf("every caller passes a literal: got (%v, %v), want elsewhere", got, ok)
		}
	}
	if edge, w := build(*lit, *req); true {
		if got, ok := w.paramOriginFromCallers("u", edge, 0); !ok || got != requestOriginRequest {
			t.Errorf("one caller passes the request: got (%v, %v), want request", got, ok)
		}
	}
	if edge, w := build(*lit, *roIdent(meta, "x", "", "app")); true {
		if _, ok := w.paramOriginFromCallers("u", edge, 0); ok {
			t.Error("a caller passing something unplaceable must give no answer")
		}
	}
	if edge, w := build(*lit); true {
		if _, ok := w.paramOriginFromCallers("other", edge, 0); ok {
			t.Error("a name no caller binds is not a parameter")
		}
		if _, ok := w.paramOriginFromCallers("u", edge, maxRequestOriginHops); ok {
			t.Error("the hop bound must end the walk")
		}
	}
	w := requestOriginWalker{cp: NewContextProvider(meta)}
	if _, ok := w.paramOriginFromCallers("u", &metadata.CallGraphEdge{Caller: call("nobody", "1")}, 0); ok {
		t.Error("a function nothing calls has no callers to agree")
	}
	f := callSiteFrame{edge: &metadata.CallGraphEdge{}}
	if f.GetKey() != "" || f.GetParent() != nil || f.GetChildren() != nil || f.GetArgument() != nil || f.GetTypeParamMap() != nil {
		t.Error("a call-site frame has no tree around it")
	}
}

// A *http.Request is not necessarily THE request: a variable's assignment and a
// parameter's callers are asked before its type. And a method handed the
// request — as an argument, or anywhere up the chain — builds from it whatever
// it is called on (review of #553).
func TestRequestOriginResolvesBeforeType(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	p := roMatcher(meta)

	// req, _ := http.NewRequest(…) — typed as a request, built as an outbound one.
	newReq := metadata.NewCallArgument(meta)
	newReq.SetKind(metadata.KindCall)
	newReq.Fun = roIdent(meta, "NewRequest", "", "net/http")
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"https://x?sig=1"`)
	newReq.Args = []*metadata.CallArgument{lit}
	urlOf := func(owner *metadata.CallArgument) *metadata.CallArgument {
		s := metadata.NewCallArgument(meta)
		s.SetKind(metadata.KindSelector)
		s.X, s.Sel = owner, roIdent(meta, "URL", "", "")
		return s
	}
	outbound := &metadata.CallGraphEdge{
		ChainParent: &metadata.CallGraphEdge{
			Receiver:      urlOf(roIdent(meta, "req", "*net/http.Request", "app")),
			AssignmentMap: map[string][]metadata.Assignment{"req": {{Value: *newReq}}},
		},
	}
	// The assignment lives on the root link, which is the frame here.
	root := outbound.ChainParent
	if !p.readsOutsideRequest(&fakeNode{edge: root}) {
		t.Error("req.URL off an http.NewRequest was taken for the request because of its type")
	}

	// reader{}.values(r).Get: the chain roots at a literal, but its link was
	// handed the request.
	rdr := metadata.NewCallArgument(meta)
	rdr.SetKind(metadata.KindCompositeLit)
	values := &metadata.CallGraphEdge{Receiver: rdr, Args: []*metadata.CallArgument{roIdent(meta, "r", "*net/http.Request", "app")}}
	if p.readsOutsideRequest(&fakeNode{edge: &metadata.CallGraphEdge{ChainParent: values}}) {
		t.Error("a chain link handed the request was dropped")
	}

	// v := reader{}.values(r); v.Get — the same through a variable.
	method := metadata.NewCallArgument(meta)
	method.SetKind(metadata.KindCall)
	method.Fun = metadata.NewCallArgument(meta)
	method.Fun.SetKind(metadata.KindSelector)
	method.Fun.X, method.Fun.Sel = rdr, roIdent(meta, "values", "", "")
	method.Fun.ReceiverType = roIdent(meta, "reader", "reader", "app")
	method.Args = []*metadata.CallArgument{roIdent(meta, "r", "*net/http.Request", "app")}
	viaVar := &metadata.CallGraphEdge{
		Receiver:      roIdent(meta, "v", "net/url.Values", "app"),
		AssignmentMap: map[string][]metadata.Assignment{"v": {{Value: *method}}},
	}
	if p.readsOutsideRequest(&fakeNode{edge: viaVar}) {
		t.Error("a method result handed the request was dropped")
	}
}
