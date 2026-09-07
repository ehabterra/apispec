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

// comboMeta describes a builder type whose second field carries the path, which
// is what a positional constructor literal has to be matched against.
func comboMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	return &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"example.com/app": {
				Name: sp.Get("app"),
				// In FILE types: TypeInPackage indexes pkg.Files[*].Types and
				// never the package-level map, so this mirrors what generation
				// actually produces.
				Files: map[string]*metadata.File{
					"app.go": {
						Types: map[string]*metadata.Type{
							"Combo": {
								Name: sp.Get("Combo"),
								Fields: []metadata.Field{
									{Name: sp.Get("r"), Type: sp.Get("*example.com/app.Router")},
									{Name: sp.Get("pattern"), Type: sp.Get("string")},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestStructFieldIndex(t *testing.T) {
	meta := comboMeta()
	for _, tc := range []struct {
		field string
		want  int
		ok    bool
	}{
		{"r", 0, true},
		{"pattern", 1, true},
		{"absent", 0, false},
	} {
		got, ok := structFieldIndex(meta, "example.com/app", "Combo", tc.field)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("structFieldIndex(%q) = (%d, %v), want (%d, %v)", tc.field, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := structFieldIndex(meta, "example.com/app", "Missing", "pattern"); ok {
		t.Error("an unknown type must not resolve a field index")
	}
	if _, ok := structFieldIndex(nil, "example.com/app", "Combo", "pattern"); ok {
		t.Error("nil metadata must not resolve a field index")
	}
}

// A constructor almost always writes its literal POSITIONALLY
// (`&Combo{r, pattern}`), so the element is found by matching the index against
// the struct's declared field order — the fact that makes the builder shape
// resolvable at all (issue #461).
func TestLiteralFieldElementPositional(t *testing.T) {
	meta := comboMeta()
	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)
	first := metadata.NewCallArgument(meta)
	first.Kind = meta.StringPool.Get(metadata.KindIdent)
	first.Name = meta.StringPool.Get("r")
	second := metadata.NewCallArgument(meta)
	second.Kind = meta.StringPool.Get(metadata.KindIdent)
	second.Name = meta.StringPool.Get("pattern")
	lit.Args = []*metadata.CallArgument{first, second}

	got, ok := literalFieldElement(meta, lit, "pattern", "example.com/app", "Combo")
	if !ok || got == nil {
		t.Fatalf("positional element for %q not found", "pattern")
	}
	if got.GetName() != "pattern" {
		t.Errorf("resolved element is %q, want the second element", got.GetName())
	}

	// A field the struct does not declare cannot be positioned.
	if _, ok := literalFieldElement(meta, lit, "absent", "example.com/app", "Combo"); ok {
		t.Error("a field the struct does not declare must not resolve")
	}
}

// A keyed literal (`&Combo{r: r, pattern: pattern}`) is matched by key, and a
// field ABSENT from the keys is the zero value — which is not a path, so it is
// reported unresolved rather than as the empty string.
func TestLiteralFieldElementKeyed(t *testing.T) {
	meta := comboMeta()
	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)

	kv := metadata.NewCallArgument(meta)
	kv.Kind = meta.StringPool.Get(metadata.KindKeyValue)
	key := metadata.NewCallArgument(meta)
	key.Kind = meta.StringPool.Get(metadata.KindIdent)
	key.Name = meta.StringPool.Get("pattern")
	val := metadata.NewCallArgument(meta)
	val.Kind = meta.StringPool.Get(metadata.KindLiteral)
	val.Value = meta.StringPool.Get(`"/items"`)
	kv.X, kv.Fun = key, val
	lit.Args = []*metadata.CallArgument{kv}

	got, ok := literalFieldElement(meta, lit, "pattern", "example.com/app", "Combo")
	if !ok || got == nil {
		t.Fatal("keyed element not found")
	}
	if got.GetValue() != `"/items"` {
		t.Errorf("resolved value %q, want %q", got.GetValue(), `"/items"`)
	}

	if _, ok := literalFieldElement(meta, lit, "r", "example.com/app", "Combo"); ok {
		t.Error("a field absent from a KEYED literal is the zero value, which is not a path")
	}
}

// The rung declines anything that is not a field read off an identifier, before
// it walks any chain.
func TestReceiverFieldValueGuards(t *testing.T) {
	meta := comboMeta()
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))

	notSelector := metadata.NewCallArgument(meta)
	notSelector.Kind = meta.StringPool.Get(metadata.KindIdent)
	notSelector.Name = meta.StringPool.Get("p")

	sel := metadata.NewCallArgument(meta)
	sel.Kind = meta.StringPool.Get(metadata.KindSelector)
	base := metadata.NewCallArgument(meta)
	base.Kind = meta.StringPool.Get(metadata.KindIdent)
	base.Name = meta.StringPool.Get("c")
	field := metadata.NewCallArgument(meta)
	field.Name = meta.StringPool.Get("pattern")
	sel.X, sel.Sel = base, field

	for name, arg := range map[string]*metadata.CallArgument{
		"nil argument":   nil,
		"not a selector": notSelector,
		"selector":       sel, // with a nil node
	} {
		if v, ok := b.receiverFieldValue(arg, nil); ok {
			t.Errorf("%s: resolved %q with no tracker node", name, v)
		}
	}
}
