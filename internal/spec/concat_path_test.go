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

// concatLit builds a string-literal CallArgument.
func concatLit(meta *metadata.Metadata, value string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindLiteral)
	a.SetValue(`"` + value + `"`)
	return a
}

// concatOf builds `left + right`.
func concatOf(meta *metadata.Metadata, left, right *metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindBinary)
	a.SetValue("+")
	a.X = left
	a.Fun = right
	return a
}

// TestResolveConcatenatedPath covers the path folding that generated servers
// need (issue #274): before it, a registration whose path is built by
// concatenation resolved to nothing at all, so the route was documented at its
// mount prefix alone — losing the only part of the path written in the source.
func TestResolveConcatenatedPath(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("two literals fold", func(t *testing.T) {
		arg := concatOf(meta, concatLit(meta, "/api"), concatLit(meta, "/things"))
		if path, dyn := b.resolvePathArg(arg, nil); path != "/api/things" || dyn != "" {
			t.Errorf("got (%q, %q), want (\"/api/things\", \"\")", path, dyn)
		}
	})

	t.Run("three operands fold left to right", func(t *testing.T) {
		inner := concatOf(meta, concatLit(meta, "/api"), concatLit(meta, "/v2"))
		arg := concatOf(meta, inner, concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, nil); path != "/api/v2/things" {
			t.Errorf("got %q, want \"/api/v2/things\"", path)
		}
	})

	t.Run("an empty operand contributes nothing", func(t *testing.T) {
		// The generated shape: BaseURL is left at its zero value, so the path is
		// exactly the literal.
		arg := concatOf(meta, concatLit(meta, ""), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, nil); path != "/things" {
			t.Errorf("got %q, want \"/things\"", path)
		}
	})

	t.Run("an unresolvable operand becomes a placeholder, keeping the literal part", func(t *testing.T) {
		// Dropping the segment would claim the handler answers somewhere it does
		// not; the placeholder keeps the route addressable and visibly partial.
		unknown := metadata.NewCallArgument(meta)
		unknown.SetKind(metadata.KindCall)
		unknown.SetName("dynamicBase")

		arg := concatOf(meta, unknown, concatLit(meta, "/dyn"))
		path, dyn := b.resolvePathArg(arg, nil)
		if path != "{dynamicBase}/dyn" {
			t.Errorf("got %q, want \"{dynamicBase}/dyn\"", path)
		}
		if dyn != "dynamicBase" {
			t.Errorf("dynamic name = %q, want \"dynamicBase\" so the caller can register the parameter", dyn)
		}
	})

	t.Run("a non-concat operator is not folded", func(t *testing.T) {
		// Only `+` builds a path. Anything else is one opaque value.
		arg := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		arg.SetValue("-")
		arg.SetName("weird")
		if path, _ := b.resolvePathArg(arg, nil); path != "{weird}" {
			t.Errorf("got %q, want the whole expression as one placeholder", path)
		}
	})

	t.Run("a plain literal path is unchanged", func(t *testing.T) {
		if path, dyn := b.resolvePathArg(concatLit(meta, "/users"), nil); path != "/users" || dyn != "" {
			t.Errorf("got (%q, %q), want (\"/users\", \"\")", path, dyn)
		}
	})
}

// TestFlattenConcat pins the operand walk itself.
func TestFlattenConcat(t *testing.T) {
	meta := newTestMeta()

	t.Run("nil is no operands", func(t *testing.T) {
		if got := flattenConcat(nil, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("a leaf is one operand", func(t *testing.T) {
		if got := flattenConcat(concatLit(meta, "/x"), nil); len(got) != 1 {
			t.Errorf("got %d operands, want 1", len(got))
		}
	})

	t.Run("a nested concat flattens in source order", func(t *testing.T) {
		inner := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		got := flattenConcat(concatOf(meta, inner, concatLit(meta, "/c")), nil)
		if len(got) != 3 {
			t.Fatalf("got %d operands, want 3", len(got))
		}
		want := []string{`"/a"`, `"/b"`, `"/c"`}
		for i, op := range got {
			if op.GetValue() != want[i] {
				t.Errorf("operand %d = %q, want %q — source order decides the path", i, op.GetValue(), want[i])
			}
		}
	})

	t.Run("any other operator abandons the whole chain", func(t *testing.T) {
		arg := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		arg.SetValue("*")
		if got := flattenConcat(arg, nil); got != nil {
			t.Errorf("got %v, want nil so the caller falls back to a placeholder", got)
		}
	})
}
