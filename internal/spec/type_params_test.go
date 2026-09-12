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

// typeParamNode is a node whose only interesting property is its bindings.
type typeParamNode struct {
	TrackerNodeInterface
	bindings map[string]string
	parent   TrackerNodeInterface
	edge     *metadata.CallGraphEdge
}

func (n *typeParamNode) GetTypeParamMap() map[string]string { return n.bindings }
func (n *typeParamNode) GetParent() TrackerNodeInterface    { return n.parent }
func (n *typeParamNode) GetEdge() *metadata.CallGraphEdge   { return n.edge }

func TestResolveTypeParam(t *testing.T) {
	node := &typeParamNode{bindings: map[string]string{
		"Res": "example.com/app.User",
		"Req": "example.com/app.CreateUserRequest",
	}}

	cases := []struct {
		name, in, want string
		isParam        bool
	}{
		// The bare parameter, in both the internal and the go/types spelling.
		{"bound parameter", "example.com/app-->Res", "example.com/app.User", true},
		{"bare name", "Res", "example.com/app.User", true},
		{"the other parameter", "Req", "example.com/app.CreateUserRequest", true},

		// Substitution is structural, so a parameter under a constructor comes
		// out whole rather than being flattened to its core.
		{"slice of the parameter", "[]Res", "[]example.com/app.User", true},
		{"pointer to the parameter", "*Res", "*example.com/app.User", true},

		// Not a type parameter: returned unchanged, and reported as such so the
		// caller does not discard what it already resolved.
		{"a real type", "example.com/app.User", "example.com/app.User", false},
		{"a primitive", "string", "string", false},
		{"empty", "", "", false},
		// A name that merely LOOKS like one is not treated as one — only a
		// binding (or a declaration, see below) makes it a parameter.
		{"unbound name", "Other", "Other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, isParam := resolveTypeParam(tc.in, node, nil)
			if got != tc.want || isParam != tc.isParam {
				t.Errorf("resolveTypeParam(%q) = (%q, %v), want (%q, %v)",
					tc.in, got, isParam, tc.want, tc.isParam)
			}
		})
	}

	// A nil node has no bindings to read and must not panic.
	if got, isParam := resolveTypeParam("Res", nil, nil); got != "Res" || isParam {
		t.Errorf("with no node: (%q, %v), want (\"Res\", false)", got, isParam)
	}
}

// An unresolved type parameter must never reach the mapper as a NAME: a
// component named after it describes nothing and is shared by every operation
// the adapter builds, which asserts that unrelated endpoints return the same
// type. The honest general type maps inline, with no $ref (issue #367,
// golden rule #7).
func TestResolveTypeParamFallsBackToTheGeneralType(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			"example.com/app": {Files: map[string]*metadata.File{
				"f.go": {Functions: map[string]*metadata.Function{
					"HandleJSON": {TypeParams: []string{"Req", "Res"}},
				}},
			}},
		},
	}
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Pkg: pool.Get("example.com/app"), Name: pool.Get("HandleJSON")},
		Callee: metadata.Call{Meta: meta, Pkg: pool.Get("example.com/app"), Name: pool.Get("Encode")},
	}
	// No bindings at all: the instantiation could not be recovered.
	node := &typeParamNode{bindings: map[string]string{}, edge: edge}

	got, isParam := resolveTypeParam("Res", node, meta)
	if !isParam {
		t.Fatal("Res is declared as a type parameter of the enclosing function, so it must be " +
			"recognised as one even with no binding")
	}
	if got != unresolvedTypeParamType {
		t.Errorf("unresolved type parameter = %q, want %q — a $ref to the parameter's name is worse "+
			"than an inline unknown, because it claims unrelated operations share a type",
			got, unresolvedTypeParamType)
	}

	// A name the enclosing function does NOT declare is left alone, so this
	// fallback cannot swallow a real type whose binding is simply absent.
	if got, isParam := resolveTypeParam("example.com/app.User", node, meta); isParam || got != "example.com/app.User" {
		t.Errorf("a declared type came back as (%q, %v), want it untouched", got, isParam)
	}

	// The declaration is looked up through the chain, because the value is
	// written inside the closure the generic function returns — whose own
	// record declares no type parameters.
	child := &typeParamNode{bindings: map[string]string{}, parent: node}
	if got, isParam := resolveTypeParam("Res", child, meta); !isParam || got != unresolvedTypeParamType {
		t.Errorf("from a child node: (%q, %v), want the fallback — the declaration is an ancestor's",
			got, isParam)
	}
}

// The general type must map to an inline schema and never to a $ref, or the
// fallback above would trade one bogus component for another.
func TestUnresolvedTypeParamTypeMapsInline(t *testing.T) {
	schema := mapGoTypeForRoute(map[string]*Schema{}, unresolvedTypeParamType, nil, &APISpecConfig{})
	if schema == nil {
		t.Fatal("the fallback type mapped to no schema at all")
	}
	if schema.Ref != "" {
		t.Errorf("the fallback type mapped to a $ref (%q); it must be inline", schema.Ref)
	}
	if schema.Type != "object" {
		t.Errorf("the fallback schema is %+v, want an inline object", schema)
	}
}
