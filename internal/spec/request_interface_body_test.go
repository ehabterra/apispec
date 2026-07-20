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

// TestRequestResolveTypeOriginInterface covers issue #164: the request path now
// applies the same interface→concrete resolution the response path already did.
//
// Two things had to be true for this to work, and both are asserted here. The
// resolvers must be reachable from the request matcher at all (they used to hang
// off the response matcher), and the argument must be peeled first: a request
// body is decoded into a POINTER (`Decode(&a)`), so the variable is the unary's
// operand, whereas the response encodes the value itself (`Encode(a)`). Without
// the peel the ident-keyed lookup never fires and the bare interface survives.
func TestRequestResolveTypeOriginInterface(t *testing.T) {
	meta := sweepInterfaceMeta()
	pool := meta.StringPool
	appFile := meta.Packages["app"].Files["app/main.go"]
	appFile.Functions["handler"] = &metadata.Function{
		AssignmentMap: map[string][]metadata.Assignment{
			"a": {{ConcreteType: pool.Get("app.Dog")}},
		},
	}
	appFile.Functions["ambiguous"] = &metadata.Function{
		AssignmentMap: map[string][]metadata.Assignment{
			"a": {
				{ConcreteType: pool.Get("app.Dog")},
				{ConcreteType: pool.Get("app.Cat")},
			},
		},
	}

	m := NewRequestPatternMatcher(RequestBodyPattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)

	// unary wraps an ident the way `&a` does in Decode(&a).
	unaryOf := func(name string) *metadata.CallArgument {
		u := metadata.NewCallArgument(meta)
		u.SetKind(metadata.KindUnary)
		u.X = sweepIdent(meta, name)
		return u
	}

	for _, tc := range []struct {
		name, caller, original string
		arg                    *metadata.CallArgument
		want                   string
	}{
		{
			name:     "pointer-to-interface resolves to the concrete",
			caller:   "handler",
			original: "app.Animal",
			arg:      unaryOf("a"),
			want:     "app.Dog",
		},
		{
			// The response shape (bare ident) must keep working through the
			// shared resolver too.
			name:     "bare ident resolves to the concrete",
			caller:   "handler",
			original: "app.Animal",
			arg:      sweepIdent(meta, "a"),
			want:     "app.Dog",
		},
		{
			name:     "ambiguous keeps the interface",
			caller:   "ambiguous",
			original: "app.Animal",
			arg:      unaryOf("a"),
			want:     "app.Animal",
		},
		{
			// A concrete body type must pass through untouched: the resolver is
			// gated on the original type being an interface.
			name:     "concrete type is untouched",
			caller:   "handler",
			original: "app.Dog",
			arg:      unaryOf("a"),
			want:     "app.Dog",
		},
		{
			name:     "unknown variable keeps the interface",
			caller:   "handler",
			original: "app.Animal",
			arg:      unaryOf("nosuch"),
			want:     "app.Animal",
		},
		{
			name:     "unary with no operand keeps the interface",
			caller:   "handler",
			original: "app.Animal",
			arg: func() *metadata.CallArgument {
				u := metadata.NewCallArgument(meta)
				u.SetKind(metadata.KindUnary)
				return u
			}(),
			want: "app.Animal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edge := sweepEdge(meta, tc.caller, "app", "Decode", "json", "", "")
			node := sweepNode(edge)
			if got := m.resolveTypeOrigin(tc.arg, node, tc.original); got != tc.want {
				t.Errorf("resolveTypeOrigin = %q, want %q", got, tc.want)
			}
		})
	}
}
