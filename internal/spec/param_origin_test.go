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
	"reflect"
	"regexp"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// originMeta builds metadata holding one function whose assignments mirror the
// two variable-receiver shapes: `rh := r.Header` (a field of the request) and
// `wh := w.Header()` (a call on the response writer).
func originMeta() *metadata.Metadata {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	ident := func(name, typ string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		a.SetType(typ)
		return a
	}
	// r.Header — a selector on the request. The types are the ones real metadata
	// records for this shape (see testdata/response_header_param).
	reqHeader := metadata.NewCallArgument(m)
	reqHeader.SetKind(metadata.KindSelector)
	reqHeader.SetType("net/http.Header")
	reqHeader.X = ident("r", "*net/http.Request")
	reqHeader.Sel = ident("Header", "net/http.Header")

	// w.Header() — a call whose function is a selector on the response writer. The
	// call carries its result type; the selector in between carries the method
	// signature.
	respHeaderFun := metadata.NewCallArgument(m)
	respHeaderFun.SetKind(metadata.KindSelector)
	respHeaderFun.SetType("func() net/http.Header")
	respHeaderFun.X = ident("w", "net/http.ResponseWriter")
	respHeaderFun.Sel = ident("Header", "func() net/http.Header")
	respHeader := metadata.NewCallArgument(m)
	respHeader.SetKind(metadata.KindCall)
	respHeader.SetType("net/http.Header")
	respHeader.Fun = respHeaderFun

	m.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{
			"main.go": {Functions: map[string]*metadata.Function{
				"handler": {AssignmentMap: map[string][]metadata.Assignment{
					"rh": {{Value: *reqHeader}},
					"wh": {{Value: *respHeader}},
				}},
			}},
		}},
	}
	return m
}

// headerGetEdge builds the `Get` edge for a header read, optionally chained onto
// a parent call with the given receiver, optionally reading through a variable.
func headerGetEdge(m *metadata.Metadata, recvVar string, parentRecvPkg, parentRecvType string) *metadata.CallGraphEdge {
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Name: m.StringPool.Get("handler"), Pkg: m.StringPool.Get("app"), Meta: m},
		Callee: metadata.Call{
			Name:     m.StringPool.Get("Get"),
			Pkg:      m.StringPool.Get("net/http"),
			RecvType: m.StringPool.Get("Header"),
			Meta:     m,
		},
		CalleeVarName: recvVar,
	}
	if parentRecvType != "" {
		edge.ChainParent = &metadata.CallGraphEdge{
			Callee: metadata.Call{
				Name:     m.StringPool.Get("Header"),
				Pkg:      m.StringPool.Get(parentRecvPkg),
				RecvType: m.StringPool.Get(parentRecvType),
				Meta:     m,
			},
		}
	}
	return edge
}

// TestReceiverOriginTypes pins what the walk reports for each shape a header read
// takes. The types it returns are the whole basis for telling a request header
// from a response one, since net/http.Header is both.
func TestReceiverOriginTypes(t *testing.T) {
	m := originMeta()
	cp := NewContextProvider(m)

	tests := []struct {
		name string
		edge *metadata.CallGraphEdge
		want []string
	}{
		{
			// w.Header().Get(k) — the chain parent states the writer.
			name: "chained on the net/http writer",
			edge: headerGetEdge(m, "", "net/http", "ResponseWriter"),
			want: []string{"net/http.ResponseWriter"},
		},
		{
			// c.Response().Header().Get(k) — echo's writer is its own type, and the
			// receiver renders with the star after the dot.
			name: "chained on the echo response",
			edge: headerGetEdge(m, "", "github.com/labstack/echo/v4", "*Response"),
			want: []string{"github.com/labstack/echo/v4.*Response"},
		},
		{
			// wh := w.Header(); wh.Get(k) — no chain parent, so the origin is the
			// assignment.
			name: "variable assigned from the writer",
			edge: headerGetEdge(m, "wh", "", ""),
			// The call's result type comes first, then what it was called on.
			want: []string{"net/http.Header", "net/http.ResponseWriter"},
		},
		{
			name: "variable assigned from the request",
			edge: headerGetEdge(m, "rh", "", ""),
			want: []string{"net/http.Header", "*net/http.Request"},
		},
		{
			// r.Header.Get(k): a field selector is not a call, so the read has no
			// chain parent and no receiver variable. Nothing is known — and the
			// caller must keep the parameter in that case.
			name: "unresolvable origin",
			edge: headerGetEdge(m, "", "", ""),
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := receiverOriginTypes(cp, tc.edge)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("receiverOriginTypes = %v, want %v", got, tc.want)
			}
		})
	}

	if got := receiverOriginTypes(cp, nil); got != nil {
		t.Errorf("nil edge = %v, want nil", got)
	}
	if got := receiverOriginTypes(nil, headerGetEdge(m, "wh", "", "")); got != nil {
		t.Errorf("nil provider = %v, want nil", got)
	}
}

// TestResponseWriterOriginRegex checks the shipped regex against the receiver
// spellings the walk actually produces — a callee receiver renders as `pkg.*T`
// while go/types renders `*pkg.T`, and both reach this regex.
func TestResponseWriterOriginRegex(t *testing.T) {
	re := regexp.MustCompile(responseWriterOriginRegex)

	for _, origin := range []string{
		"net/http.ResponseWriter",
		"net/http.*ResponseWriter",
		"github.com/labstack/echo/v4.*Response",
		"*github.com/labstack/echo/v4.Response",
		"github.com/gin-gonic/gin.ResponseWriter",
		"github.com/gin-gonic/gin.*ResponseWriter",
	} {
		if !re.MatchString(origin) {
			t.Errorf("%q is a response writer but does not match", origin)
		}
	}

	// Request-side and unrelated origins must not match, or real parameters are
	// dropped.
	for _, origin := range []string{
		"*net/http.Request",
		"net/http.Request",
		"github.com/labstack/echo/v4.Context",
		"net/http.Header",
		// A user type merely named like the response, in another package.
		"example.com/app.ResponseWriterConfig",
		"example.com/app.Response",
	} {
		if re.MatchString(origin) {
			t.Errorf("%q matched the response-writer regex; the parameter would be dropped", origin)
		}
	}
}

// TestParamMatchNodeExcludesResponseOrigin pins the matcher-level rule: a pattern
// carrying the exclusion rejects a proven response origin and keeps everything
// else, including origins it cannot resolve (golden rule #7).
func TestParamMatchNodeExcludesResponseOrigin(t *testing.T) {
	m := originMeta()
	pattern := ParamPattern{
		CallRegex:              "^Get$",
		ParamIn:                "header",
		RecvType:               "net/http.Header",
		ExcludeRecvOriginRegex: responseWriterOriginRegex,
	}
	matcher := NewParamPatternMatcher(pattern, &APISpecConfig{}, NewContextProvider(m), nil)

	tests := []struct {
		name  string
		edge  *metadata.CallGraphEdge
		match bool
	}{
		{"response writer chain", headerGetEdge(m, "", "net/http", "ResponseWriter"), false},
		{"echo response chain", headerGetEdge(m, "", "github.com/labstack/echo/v4", "*Response"), false},
		{"writer through a variable", headerGetEdge(m, "wh", "", ""), false},
		{"request through a variable", headerGetEdge(m, "rh", "", ""), true},
		{"origin unknown", headerGetEdge(m, "", "", ""), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := &TrackerNode{CallGraphEdge: tc.edge}
			if got := matcher.MatchNode(node); got != tc.match {
				t.Errorf("MatchNode = %v, want %v", got, tc.match)
			}
		})
	}

	// Without the exclusion configured, nothing is filtered — the field is opt-in
	// per pattern.
	plain := NewParamPatternMatcher(
		ParamPattern{CallRegex: "^Get$", ParamIn: "header", RecvType: "net/http.Header"},
		&APISpecConfig{}, NewContextProvider(m), nil,
	)
	node := &TrackerNode{CallGraphEdge: headerGetEdge(m, "", "net/http", "ResponseWriter")}
	if !plain.MatchNode(node) {
		t.Error("a pattern without ExcludeRecvOriginRegex filtered a match")
	}
}
