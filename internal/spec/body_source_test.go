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

// newTestMeta returns an empty Metadata with a usable string pool.
func newTestMeta() *metadata.Metadata {
	return &metadata.Metadata{StringPool: metadata.NewStringPool()}
}

// mkIdent builds an ident CallArgument with the given name and type.
func mkIdent(meta *metadata.Metadata, name, typ string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	if typ != "" {
		a.Type = meta.StringPool.Get(typ)
	}
	return a
}

// mkSelector builds `x.sel` as a CallArgument.
func mkSelector(meta *metadata.Metadata, x, sel *metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindSelector)
	a.X = x
	a.Sel = sel
	return a
}

// mkMethodCall builds `x.method()` as a KindCall whose Fun is a selector.
func mkMethodCall(meta *metadata.Metadata, x, method *metadata.CallArgument) *metadata.CallArgument {
	fun := mkSelector(meta, x, method)
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindCall)
	a.Fun = fun
	return a
}

func TestPeelAccessorChain(t *testing.T) {
	meta := newTestMeta()

	t.Run("r.Body", func(t *testing.T) {
		root := mkIdent(meta, "r", "*net/http.Request")
		expr := mkSelector(meta, root, mkIdent(meta, "Body", ""))
		got, segs := peelAccessorChain(expr)
		if got != root {
			t.Fatalf("root mismatch: got %v want %v", got, root)
		}
		if accessorString(segs) != "Body" {
			t.Fatalf("accessor: got %q want %q", accessorString(segs), "Body")
		}
	})

	t.Run("c.Request.Body", func(t *testing.T) {
		root := mkIdent(meta, "c", "*gin.Context")
		req := mkSelector(meta, root, mkIdent(meta, "Request", ""))
		expr := mkSelector(meta, req, mkIdent(meta, "Body", ""))
		got, segs := peelAccessorChain(expr)
		if got != root {
			t.Fatalf("root mismatch")
		}
		if accessorString(segs) != "Request.Body" {
			t.Fatalf("accessor: got %q", accessorString(segs))
		}
	})

	t.Run("c.Request().Body", func(t *testing.T) {
		root := mkIdent(meta, "c", "echo.Context")
		reqCall := mkMethodCall(meta, root, mkIdent(meta, "Request", ""))
		expr := mkSelector(meta, reqCall, mkIdent(meta, "Body", ""))
		got, segs := peelAccessorChain(expr)
		if got != root {
			t.Fatalf("root mismatch")
		}
		if accessorString(segs) != "Request().Body" {
			t.Fatalf("accessor: got %q", accessorString(segs))
		}
	})

	t.Run("c.Body()", func(t *testing.T) {
		root := mkIdent(meta, "c", "*fiber.Ctx")
		expr := mkMethodCall(meta, root, mkIdent(meta, "Body", ""))
		got, segs := peelAccessorChain(expr)
		if got != root {
			t.Fatalf("root mismatch")
		}
		if accessorString(segs) != "Body()" {
			t.Fatalf("accessor: got %q", accessorString(segs))
		}
	})

	t.Run("nonChain", func(t *testing.T) {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		root, segs := peelAccessorChain(lit)
		if root != nil || segs != nil {
			t.Fatalf("expected (nil,nil) for literal, got (%v,%v)", root, segs)
		}
	})
}

// TestBodySourceResolver_ChainMatches verifies that the resolver classifies
// (request-context typed root, body accessor) chains as request sources
// and rejects everything else, for each framework preset.
func TestBodySourceResolver_ChainMatches(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	cases := []struct {
		name string
		cfg  RequestContextConfig
		root *metadata.CallArgument
		expr *metadata.CallArgument
		want bool
	}{
		{
			name: "net/http: r.Body",
			cfg:  netHTTPRequestContext,
			root: mkIdent(meta, "r", "*net/http.Request"),
			want: true,
		},
		{
			name: "net/http: r.Body when typed without star",
			cfg:  netHTTPRequestContext,
			root: mkIdent(meta, "r", "net/http.Request"),
			want: true,
		},
		{
			name: "net/http: foo.Body on unrelated type",
			cfg:  netHTTPRequestContext,
			root: mkIdent(meta, "foo", "*os.File"),
			want: false,
		},
		{
			name: "gin: c.Request.Body",
			cfg:  ginRequestContext,
			root: mkIdent(meta, "c", "*github.com/gin-gonic/gin.Context"),
			want: true,
		},
		{
			name: "fiber: c.Body() (method)",
			cfg:  fiberRequestContext,
			root: mkIdent(meta, "c", "*github.com/gofiber/fiber/v2.Ctx"),
			want: true,
		},
		{
			name: "echo: c.Request().Body",
			cfg:  echoRequestContext,
			root: mkIdent(meta, "c", "github.com/labstack/echo/v4.Context"),
			want: true,
		},
	}

	// Build the body expression for each case based on its framework shape.
	buildExpr := func(name string, root *metadata.CallArgument) *metadata.CallArgument {
		switch name {
		case "net/http: r.Body", "net/http: r.Body when typed without star", "net/http: foo.Body on unrelated type":
			return mkSelector(meta, root, mkIdent(meta, "Body", ""))
		case "gin: c.Request.Body":
			return mkSelector(meta, mkSelector(meta, root, mkIdent(meta, "Request", "")), mkIdent(meta, "Body", ""))
		case "fiber: c.Body() (method)":
			return mkMethodCall(meta, root, mkIdent(meta, "Body", ""))
		case "echo: c.Request().Body":
			return mkSelector(meta, mkMethodCall(meta, root, mkIdent(meta, "Request", "")), mkIdent(meta, "Body", ""))
		}
		return nil
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &APISpecConfig{Framework: FrameworkConfig{RequestContext: tc.cfg}}
			r := newBodySourceResolver(cfg, cp)
			if !r.Enabled() {
				t.Fatalf("resolver should be enabled with TypeRegexes=%v", tc.cfg.TypeRegexes)
			}
			expr := buildExpr(tc.name, tc.root)
			if expr == nil {
				t.Fatalf("missing test setup for %q", tc.name)
			}
			edge := &metadata.CallGraphEdge{}
			got := r.IsRequestSource(expr, edge)
			if got != tc.want {
				t.Fatalf("IsRequestSource(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestBodySourceResolver_Disabled returns true (permissive) when no
// RequestContext is configured, so existing behaviour is preserved.
func TestBodySourceResolver_Disabled(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newBodySourceResolver(&APISpecConfig{}, cp)
	if r.Enabled() {
		t.Fatalf("expected resolver to be disabled with no TypeRegexes")
	}
	if !r.IsRequestSource(mkIdent(meta, "x", "*os.File"), &metadata.CallGraphEdge{}) {
		t.Fatalf("disabled resolver must be permissive")
	}
}
