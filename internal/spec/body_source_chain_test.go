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

// mkTypedSelector builds `x.sel` carrying the type the whole expression has, so
// a chain can be asked what type it holds PART WAY along — which is what
// distinguishes a house context's `c.Req.Body` from a client's `resp.Body`.
func mkTypedSelector(meta *metadata.Metadata, x, sel *metadata.CallArgument, typ string) *metadata.CallArgument {
	a := mkSelector(meta, x, sel)
	if typ != "" {
		a.Type = meta.StringPool.Get(typ)
	}
	return a
}

// mkTypedMethodCall builds `x.method()` carrying its result type.
func mkTypedMethodCall(meta *metadata.Metadata, x, method *metadata.CallArgument, typ string) *metadata.CallArgument {
	a := mkMethodCall(meta, x, method)
	if typ != "" {
		a.Type = meta.StringPool.Get(typ)
	}
	return a
}

// TestChainMatchesFindsRequestPastTheRoot pins which chains count as reading the
// request body (issue #513).
//
// The request is not always the root of the chain: a project's own context holds
// it in a field, so `c.Req.Body` has a root typed `*Ctx` and the request one
// accessor in. Reading the root's type alone answered "not the request" for
// every such project — invisible at extraction time, where the derived wrapper
// pattern documents the body anyway, and load-bearing in wrapper DERIVATION,
// which had to skip the check entirely to work at all and so read an outbound
// `http.Response` decode as a request body.
//
// Driven only by the configured RequestContext, so every framework's own context
// is covered by construction rather than by a branch per framework (golden rule
// #5) — hence the table below runs the shipped presets.
func TestChainMatchesFindsRequestPastTheRoot(t *testing.T) {
	meta := newTestMeta()
	edge := &metadata.CallGraphEdge{}

	// house builds `c.Req.Body` for a project's own context type: the shape the
	// issue is about, and the one that must not be confused with an outbound
	// response.
	house := func(ctxType, reqType string) *metadata.CallArgument {
		root := mkIdent(meta, "c", ctxType)
		req := mkTypedSelector(meta, root, mkIdent(meta, "Req", ""), reqType)
		return mkTypedSelector(meta, req, mkIdent(meta, "Body", ""), "io.ReadCloser")
	}

	cases := []struct {
		name string
		cfg  RequestContextConfig
		expr *metadata.CallArgument
		want bool
		why  string
	}{
		{
			name: "house context holding the request in a field",
			cfg:  netHTTPRequestContext,
			expr: house("*github.com/x/p.Ctx", "*net/http.Request"),
			want: true,
			why:  "c.Req is the request; the root being the project's own context does not change that",
		},
		{
			name: "outbound response with the identical shape",
			cfg:  netHTTPRequestContext,
			// resp.Body, where resp is what an http.Client returned.
			expr: mkTypedSelector(meta,
				mkIdent(meta, "resp", "*net/http.Response"),
				mkIdent(meta, "Body", ""), "io.ReadCloser"),
			want: false,
			why:  "an *http.Response is never a request body — the distinguishing fact is the type, not the accessor",
		},
		{
			name: "the request itself, at the root",
			cfg:  netHTTPRequestContext,
			expr: mkTypedSelector(meta,
				mkIdent(meta, "r", "*net/http.Request"),
				mkIdent(meta, "Body", ""), "io.ReadCloser"),
			want: true,
			why:  "the plain handler shape, which matched at the root before and must still",
		},
		{
			name: "house context holding a response",
			cfg:  netHTTPRequestContext,
			expr: house("*github.com/x/p.Client", "*net/http.Response"),
			want: false,
			why:  "walking past the root must not accept any field named Body — only a request-typed one",
		},
		{
			name: "a field of an unrelated type",
			cfg:  netHTTPRequestContext,
			expr: mkTypedSelector(meta,
				mkIdent(meta, "f", "*github.com/x/p.Form"),
				mkIdent(meta, "Body", ""), "string"),
			want: false,
			why:  "nothing in the chain is a request context",
		},
		{
			name: "gin, through its context's Request field",
			cfg:  ginRequestContext,
			expr: mkTypedSelector(meta,
				mkTypedSelector(meta,
					mkIdent(meta, "c", "*github.com/gin-gonic/gin.Context"),
					mkIdent(meta, "Request", ""), "*net/http.Request"),
				mkIdent(meta, "Body", ""), "io.ReadCloser"),
			want: true,
			why:  "gin matches this at the root too (Request.Body is a configured accessor); both routes must agree",
		},
		{
			name: "echo, through its context's Request() method",
			cfg:  echoRequestContext,
			expr: mkTypedSelector(meta,
				mkTypedMethodCall(meta,
					mkIdent(meta, "c", "github.com/labstack/echo/v4.Context"),
					mkIdent(meta, "Request", ""), "*net/http.Request"),
				mkIdent(meta, "Body", ""), "io.ReadCloser"),
			want: true,
			why:  "a method call part way along the chain carries a type the same as a field does",
		},
		{
			name: "a house context wrapping echo's",
			cfg:  echoRequestContext,
			// c.Ctx.Request().Body — a project's own context holding echo's,
			// which is the house shape one framework further out.
			expr: mkTypedSelector(meta,
				mkTypedMethodCall(meta,
					mkTypedSelector(meta,
						mkIdent(meta, "c", "*github.com/x/p.APICtx"),
						mkIdent(meta, "Ctx", ""), "github.com/labstack/echo/v4.Context"),
					mkIdent(meta, "Request", ""), "*net/http.Request"),
				mkIdent(meta, "Body", ""), "io.ReadCloser"),
			want: true,
			why:  "every framework's context is reachable through a house type, and the walk is driven by config alone",
		},
		{
			name: "fiber, whose body is a method on the context",
			cfg:  fiberRequestContext,
			expr: mkTypedMethodCall(meta,
				mkIdent(meta, "c", "*github.com/gofiber/fiber/v2.Ctx"),
				mkIdent(meta, "Body", ""), "[]byte"),
			want: true,
			why:  "the root-typed case, which the prefix walk must leave alone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBodySourceResolver(&APISpecConfig{
				Framework: FrameworkConfig{RequestContext: tc.cfg},
			}, NewContextProvider(meta))

			root, segs := peelAccessorChain(tc.expr)
			if root == nil {
				t.Fatalf("chain did not decompose")
			}
			if got := r.chainMatches(root, segs, edge); got != tc.want {
				t.Errorf("chainMatches(%s) = %v, want %v — %s",
					accessorString(segs), got, tc.want, tc.why)
			}
		})
	}
}

// TestChainMatchesWalksPastTheRootOnlyForGivenValues pins the provenance rule
// that keeps the chain walk from reaching a request the handler SENT.
//
// Both of these hold an *http.Request one accessor in, and only the first is
// the request being served:
//
//	c.Req.Body          // c is the receiver — the context holds our request
//	resp.Request.Body   // resp came from http.Get — the request we sent
//
// What separates them is where the root came from: a parameter or receiver is
// something the handler was given, while `resp` is a local it built. So the
// walk past the root is offered only to the former.
func TestChainMatchesWalksPastTheRootOnlyForGivenValues(t *testing.T) {
	meta := newTestMeta()
	r := newBodySourceResolver(&APISpecConfig{
		Framework: FrameworkConfig{RequestContext: netHTTPRequestContext},
	}, NewContextProvider(meta))

	// `resp, err := http.Get(…)` — what makes resp a local rather than a
	// parameter, and the only thing that tells the two chains apart.
	assigned := func(name string) *metadata.CallGraphEdge {
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		return &metadata.CallGraphEdge{
			AssignmentMap: map[string][]metadata.Assignment{
				name: {{Value: *call, CalleeFunc: "Get", CalleePkg: "net/http"}},
			},
		}
	}

	respRequestBody := func() *metadata.CallArgument {
		return mkTypedSelector(meta,
			mkTypedSelector(meta,
				mkIdent(meta, "resp", "*net/http.Response"),
				mkIdent(meta, "Request", ""), "*net/http.Request"),
			mkIdent(meta, "Body", ""), "io.ReadCloser")
	}

	t.Run("the request a response carries, from a local", func(t *testing.T) {
		root, segs := peelAccessorChain(respRequestBody())
		if r.chainMatches(root, segs, assigned("resp")) {
			t.Error("chainMatches = true; resp.Request is the request that was SENT, not the one being served")
		}
	})

	t.Run("a context field, from the receiver", func(t *testing.T) {
		// The shape the walk exists for. No assignment for `c`: it is the
		// method's receiver.
		expr := mkTypedSelector(meta,
			mkTypedSelector(meta,
				mkIdent(meta, "c", "*github.com/x/p.Ctx"),
				mkIdent(meta, "Req", ""), "*net/http.Request"),
			mkIdent(meta, "Body", ""), "io.ReadCloser")
		root, segs := peelAccessorChain(expr)
		if !r.chainMatches(root, segs, &metadata.CallGraphEdge{}) {
			t.Error("chainMatches = false; a house context holds the request in a field and must still resolve")
		}
	})

	// A LIMIT, recorded rather than guaranteed: at the root the check asks
	// whether a value is an *http.Request, never whose, so a request the
	// handler built to send is accepted. That predates the chain walk — it is
	// how `req := r; req.Body` keeps working — and closing it needs provenance
	// run to the handler's own parameter, which #513 describes in full and this
	// does not attempt.
	t.Run("an outbound request at the root is still accepted", func(t *testing.T) {
		expr := mkTypedSelector(meta,
			mkIdent(meta, "outReq", "*net/http.Request"),
			mkIdent(meta, "Body", ""), "io.ReadCloser")
		root, segs := peelAccessorChain(expr)
		if !r.chainMatches(root, segs, assigned("outReq")) {
			t.Error("chainMatches = false; if this now fails, provenance got tighter — update the comment above, it is an improvement")
		}
	})
}

// TestChainMatchesNeedsAnAccessorLeft guards the loop bound: the prefix walk
// offers every position EXCEPT the last, because a chain that is nothing but the
// request has no accessor left to identify a body.
//
// Without the bound, `r` alone (or `c.Req`) would be read as its own body and
// every request passed to a helper would look like a request body.
func TestChainMatchesNeedsAnAccessorLeft(t *testing.T) {
	meta := newTestMeta()
	r := newBodySourceResolver(&APISpecConfig{
		Framework: FrameworkConfig{RequestContext: netHTTPRequestContext},
	}, NewContextProvider(meta))

	// c.Req — the request reached through a field, with nothing after it.
	root := mkIdent(meta, "c", "*github.com/x/p.Ctx")
	expr := mkTypedSelector(meta, root, mkIdent(meta, "Req", ""), "*net/http.Request")

	gotRoot, segs := peelAccessorChain(expr)
	if gotRoot == nil {
		t.Fatalf("chain did not decompose")
	}
	if r.chainMatches(gotRoot, segs, &metadata.CallGraphEdge{}) {
		t.Errorf("chainMatches(%s) = true; the request itself is not its own body", accessorString(segs))
	}
}
