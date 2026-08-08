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
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// litArg builds a CallArgument of the given kind, with the fields the map-literal
// reader consults.
func litArg(m *metadata.Metadata, kind, value, typeStr string) *metadata.CallArgument {
	a := metadata.NewCallArgument(m)
	a.SetKind(kind)
	if value != "" {
		a.SetValue(value)
	}
	if typeStr != "" {
		a.SetType(typeStr)
	}
	return a
}

// kv builds one `key: value` entry of a map literal.
func kv(m *metadata.Metadata, key, value *metadata.CallArgument) *metadata.CallArgument {
	e := metadata.NewCallArgument(m)
	e.SetKind(metadata.KindKeyValue)
	e.X = key
	e.Fun = value
	return e
}

// mapLit builds `map[<keyType>]any{ entries... }`.
func mapLit(m *metadata.Metadata, keyType string, entries ...*metadata.CallArgument) *metadata.CallArgument {
	kt := metadata.NewCallArgument(m)
	kt.SetKind(metadata.KindIdent)
	kt.SetName(keyType)

	mt := metadata.NewCallArgument(m)
	mt.SetKind(metadata.KindMapType)
	mt.X = kt

	lit := metadata.NewCallArgument(m)
	lit.SetKind(metadata.KindCompositeLit)
	lit.X = mt
	lit.Args = entries
	return lit
}

// byType resolves a value to whatever its recorded type says, which is what the
// extractor's resolver does for the simple cases; a value with no type stands in
// for one the resolver could not narrow.
func byType(a *metadata.CallArgument) string { return a.GetType() }

// TestConstantKeyValueReadsOnlyConstantStringKeys pins which map entries name a
// property. Anything that is not a quoted string literal is left to
// additionalProperties rather than guessed at — a computed key has no name to
// document, and inventing one would claim a field the handler may never write.
func TestConstantKeyValueReadsOnlyConstantStringKeys(t *testing.T) {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	val := litArg(m, metadata.KindIdent, "", "pkg.Widget")

	t.Run("quoted key", func(t *testing.T) {
		key, got, ok := constantKeyValue(kv(m, litArg(m, metadata.KindLiteral, `"items"`, ""), val))
		if !ok || key != "items" || got != val {
			t.Errorf("key=%q ok=%v, want items/true", key, ok)
		}
	})

	// A key with an escape must be UNQUOTED, not trimmed: trimming would leave
	// the backslash in the property name.
	t.Run("escaped key", func(t *testing.T) {
		key, _, ok := constantKeyValue(kv(m, litArg(m, metadata.KindLiteral, `"a\"b"`, ""), val))
		if !ok || key != `a"b` {
			t.Errorf("key=%q ok=%v, want a\"b/true", key, ok)
		}
	})

	// Rejected: each would otherwise become a bogus property name.
	for name, key := range map[string]*metadata.CallArgument{
		"computed key (ident)": litArg(m, metadata.KindIdent, "", "string"),
		"numeric literal key":  litArg(m, metadata.KindLiteral, `42`, ""),
		"empty string key":     litArg(m, metadata.KindLiteral, `""`, ""),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := constantKeyValue(kv(m, key, val)); ok {
				t.Errorf("%s was read as a property name", name)
			}
		})
	}

	t.Run("not a key-value entry", func(t *testing.T) {
		if _, _, ok := constantKeyValue(litArg(m, metadata.KindIdent, "", "pkg.Widget")); ok {
			t.Error("a bare element is not a key-value pair")
		}
		if _, _, ok := constantKeyValue(nil); ok {
			t.Error("nil entry read as a property")
		}
	})
}

// TestMapLiteralSchemaOnlyClaimsWhatTheLiteralSays is the honesty contract: the
// keys that could be read become properties, and everything else stays possible
// rather than being denied (golden rule #7).
func TestMapLiteralSchemaOnlyClaimsWhatTheLiteralSays(t *testing.T) {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	cfg := &APISpecConfig{}
	str := func() *metadata.CallArgument { return litArg(m, metadata.KindIdent, "", "string") }

	t.Run("all keys constant", func(t *testing.T) {
		lit := mapLit(m, "string",
			kv(m, litArg(m, metadata.KindLiteral, `"b"`, ""), str()),
			kv(m, litArg(m, metadata.KindLiteral, `"a"`, ""), str()),
		)
		s := mapLiteralSchema(lit, byType, map[string]*Schema{}, m, cfg)
		if s == nil {
			t.Fatal("a fully constant literal produced no object schema")
		}
		if s.Type != "object" || len(s.Properties) != 2 {
			t.Fatalf("got %+v, want an object with 2 properties", s)
		}
		if s.AdditionalProperties != nil {
			t.Error("nothing was unreadable, so nothing is left for additionalProperties")
		}
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if keys[0] != "a" || keys[1] != "b" {
			t.Errorf("properties %v, want a and b", keys)
		}
	})

	t.Run("one unreadable key keeps the rest possible", func(t *testing.T) {
		lit := mapLit(m, "string",
			kv(m, litArg(m, metadata.KindLiteral, `"a"`, ""), str()),
			kv(m, litArg(m, metadata.KindIdent, "", "string"), str()), // computed key
		)
		s := mapLiteralSchema(lit, byType, map[string]*Schema{}, m, cfg)
		if s == nil || len(s.Properties) != 1 {
			t.Fatalf("got %+v, want the one readable key documented", s)
		}
		if s.AdditionalProperties == nil {
			t.Error("the computed key must stay possible, not be denied")
		}
	})

	// A key whose value type did not resolve is still a key: naming it with an
	// unconstrained schema beats dropping the field.
	t.Run("unresolved value keeps its key", func(t *testing.T) {
		lit := mapLit(m, "string",
			kv(m, litArg(m, metadata.KindLiteral, `"a"`, ""), litArg(m, metadata.KindIdent, "", "")),
		)
		s := mapLiteralSchema(lit, byType, map[string]*Schema{}, m, cfg)
		if s == nil || s.Properties["a"] == nil {
			t.Fatalf("got %+v, want property a present", s)
		}
	})

	// Cases that must fall through to the type-driven mapping, so the fix cannot
	// become a blanket "map literals are objects".
	t.Run("falls through", func(t *testing.T) {
		for name, arg := range map[string]*metadata.CallArgument{
			"non-string key": mapLit(m, "int",
				kv(m, litArg(m, metadata.KindLiteral, `"a"`, ""), str())),
			"empty literal":     mapLit(m, "string"),
			"not a map literal": litArg(m, metadata.KindIdent, "", "pkg.Widget"),
			"nil":               nil,
			"every key unreadable": mapLit(m, "string",
				kv(m, litArg(m, metadata.KindIdent, "", "string"), str())),
		} {
			t.Run(name, func(t *testing.T) {
				if s := mapLiteralSchema(arg, byType, map[string]*Schema{}, m, cfg); s != nil {
					t.Errorf("%s produced %+v, want nil so the type-driven mapping decides", name, s)
				}
			})
		}
	})
}
