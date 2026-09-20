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

// TestContentTypeHeaderWrite pins which header writes declare a response media
// type. A header write is not the signal; the header it NAMES is — otherwise
// `w.Header().Set("X-Request-Id", …)` would document a body.
func TestContentTypeHeaderWrite(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	cfg := DefaultChiConfig()
	m := NewResponsePatternMatcher(contentTypeResponsePattern(stdlibContentTypeWrites()), cfg, NewContextProvider(meta))

	lit := func(v string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindLiteral)
		a.SetValue(`"` + v + `"`)
		return a
	}
	set := func(recv string, args ...*metadata.CallArgument) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{
			Callee: metadata.Call{Meta: meta, Name: sp.Get("Set"), Pkg: sp.Get("net/http"), RecvType: sp.Get(recv)},
			Args:   args,
		}
	}

	t.Run("a content-type declaration", func(t *testing.T) {
		got, ok := m.contentTypeHeaderWrite(set("Header", lit("Content-Type"), lit("text/csv")))
		if !ok || got != "text/csv" {
			t.Errorf("= (%q, %v), want (text/csv, true)", got, ok)
		}
	})

	t.Run("the canonical spelling is not required", func(t *testing.T) {
		// Go canonicalises header keys, and a handler may write any case.
		if got, ok := m.contentTypeHeaderWrite(set("Header", lit("content-type"), lit("text/csv"))); !ok || got != "text/csv" {
			t.Errorf("= (%q, %v), want it matched case-insensitively", got, ok)
		}
	})

	t.Run("another header", func(t *testing.T) {
		if _, ok := m.contentTypeHeaderWrite(set("Header", lit("X-Request-Id"), lit("abc"))); ok {
			t.Error("a non-content-type header was read as a media-type declaration")
		}
	})

	t.Run("a value decided at runtime", func(t *testing.T) {
		// Not a constant: the media type is unknown, and an operation
		// documented with the WRONG content type is worse than one with none.
		runtimeVal := mkIdent(meta, "ct", "string")
		if _, ok := m.contentTypeHeaderWrite(set("Header", lit("Content-Type"), runtimeVal)); ok {
			t.Error("a runtime value was read as a declared media type")
		}
	})

	t.Run("the wrong receiver", func(t *testing.T) {
		if _, ok := m.contentTypeHeaderWrite(set("SomeMap", lit("Content-Type"), lit("text/csv"))); ok {
			t.Error("a Set on something other than a header map was matched")
		}
	})

	t.Run("too few arguments", func(t *testing.T) {
		if _, ok := m.contentTypeHeaderWrite(set("Header", lit("Content-Type"))); ok {
			t.Error("a call with no value argument was matched")
		}
		if _, ok := m.contentTypeHeaderWrite(nil); ok {
			t.Error("a nil edge was matched")
		}
	})
}

// TestFrameworkContentTypeWrites pins that a framework's shorthand is ADDED to
// the stdlib entry rather than replacing it — every router's writer is
// reachable as an http.Header somewhere (echo's Response().Header().Set,
// gin's Writer.Header().Set), so losing that entry would lose those handlers.
func TestFrameworkContentTypeWrites(t *testing.T) {
	base := len(stdlibContentTypeWrites())

	withShorthand := frameworkContentTypeWrites(`^\*?Ctx$`, `^Set$`)
	if len(withShorthand) != base+1 {
		t.Errorf("got %d entries, want the stdlib %d plus one shorthand", len(withShorthand), base)
	}

	// A framework with no shorthand of its own keeps exactly the stdlib entry.
	for _, empty := range [][2]string{{"", "^Set$"}, {`^\*?Ctx$`, ""}, {"", ""}} {
		if got := frameworkContentTypeWrites(empty[0], empty[1]); len(got) != base {
			t.Errorf("frameworkContentTypeWrites(%q, %q) = %d entries, want %d",
				empty[0], empty[1], len(got), base)
		}
	}
}

// TestAnyCallRegexComesFromConfig pins that the coarse filter is DERIVED from
// the configured writes.
//
// MatchNode applies it before ExtractResponse can inspect the header-NAME
// argument, so a hardcoded list silently discarded any project that configured
// its own setter — the calls being configuration would have meant nothing.
func TestAnyCallRegexComesFromConfig(t *testing.T) {
	re := anyCallRegex([]ContentTypeWrite{
		{CallRegex: `^Set$`},
		{CallRegex: `^SetContentType$`},
		{CallRegex: `^Set$`}, // duplicate: one alternative, not two
	})

	compiled, err := cachedRegex(re)
	if err != nil {
		t.Fatalf("derived regex does not compile: %q: %v", re, err)
	}
	for _, name := range []string{"Set", "SetContentType"} {
		if !compiled.MatchString(name) {
			t.Errorf("%q is configured and does not match %q", name, re)
		}
	}
	for _, name := range []string{"Write", "Encode", "Header"} {
		if compiled.MatchString(name) {
			t.Errorf("%q is not configured but matches %q", name, re)
		}
	}

	t.Run("no writes configured", func(t *testing.T) {
		// Must match NOTHING rather than everything: an empty alternation
		// would make the pattern claim every call it sees.
		empty, err := cachedRegex(anyCallRegex(nil))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		for _, name := range []string{"Set", "Write", ""} {
			if empty.MatchString(name) {
				t.Errorf("an unconfigured pattern matched %q", name)
			}
		}
	})

	t.Run("an entry with no call regex is skipped", func(t *testing.T) {
		if got := anyCallRegex([]ContentTypeWrite{{CallRegex: ""}}); got != matchNothingRegex {
			t.Errorf("= %q, want the match-nothing regex", got)
		}
	})
}
