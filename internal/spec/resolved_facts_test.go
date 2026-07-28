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

// TestTypeFactsFor covers the two questions that decide whether a resolved
// callee may be trusted over the recorded one.
func TestTypeFactsFor(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	meta.Packages = map[string]*metadata.Package{
		"example.com/app": {Files: map[string]*metadata.File{
			"types.go": {Types: map[string]*metadata.Type{
				"Storage": {Name: pool.Get("Storage"), Kind: pool.Get("interface")},
				"Base":    {Name: pool.Get("Base"), Kind: pool.Get("struct")},
				"Context": {Name: pool.Get("Context"), Kind: pool.Get("struct"), Embeds: []int{pool.Get("*Base")}},
				// Transitive: embeds the type that embeds Base.
				"APIContext": {Name: pool.Get("APIContext"), Kind: pool.Get("struct"), Embeds: []int{pool.Get("*Context")}},
				"Widget":     {Name: pool.Get("Widget"), Kind: pool.Get("struct")},
			}},
		}},
	}

	facts := TypeFactsFor(meta)

	if !facts.IsInterface("example.com/app.Storage") {
		t.Error("Storage is declared as an interface and was not reported as one")
	}
	if facts.IsInterface("example.com/app.Base") {
		t.Error("Base is a struct and was reported as an interface")
	}
	if facts.IsInterface("example.com/missing.Thing") {
		t.Error("a type in a package metadata does not hold was reported as an interface")
	}
	// Memoized answers must not differ from first ones.
	if !facts.IsInterface("example.com/app.Storage") {
		t.Error("the memoized answer contradicts the first")
	}

	if !facts.Embeds("example.com/app.Context", "example.com/app.Base") {
		t.Error("Context embeds Base and the promotion was not seen")
	}
	if !facts.Embeds("example.com/app.APIContext", "example.com/app.Base") {
		t.Error("APIContext embeds Base transitively — gitea's shape, 4,492 call sites")
	}
	if facts.Embeds("example.com/app.Widget", "example.com/app.Base") {
		t.Error("Widget embeds nothing and was reported as embedding Base")
	}
	if facts.Embeds("example.com/app.Base", "example.com/app.Context") {
		t.Error("embedding is directional; Base does not embed Context")
	}
	if !facts.Embeds("example.com/app.APIContext", "example.com/app.Base") {
		t.Error("the memoized answer contradicts the first")
	}
}

// TestTypeFactsForNilMetadata pins that the facts are safe when the resolved
// graph is off and nothing has been loaded.
func TestTypeFactsForNilMetadata(t *testing.T) {
	facts := TypeFactsFor(nil)
	if facts.IsInterface != nil || facts.Embeds != nil {
		t.Error("nil metadata produced non-nil fact functions; a caller would ask them and get invented answers")
	}
}

func TestSplitQualified(t *testing.T) {
	for input, want := range map[string][2]string{
		"github.com/x/y.Type": {"github.com/x/y", "Type"},
		"app.Type":            {"app", "Type"},
		"Type":                {"", ""},
		"":                    {"", ""},
	} {
		pkg, name := splitQualified(input)
		if pkg != want[0] || name != want[1] {
			t.Errorf("splitQualified(%q) = (%q, %q), want (%q, %q)", input, pkg, name, want[0], want[1])
		}
	}
}

// TestTypeFactsForUnknownAndQualifiedEmbeds covers the two shapes the embedding
// walk has to survive on a real project: a type in a package metadata never
// loaded, and an embedded name that already carries its own package.
func TestTypeFactsForUnknownAndQualifiedEmbeds(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	meta.Packages = map[string]*metadata.Package{
		"example.com/app": {Files: map[string]*metadata.File{
			"types.go": {Types: map[string]*metadata.Type{
				// Embeds a type from another package, already qualified.
				"Handler": {Name: pool.Get("Handler"), Embeds: []int{pool.Get("*other.io/pkg.Base")}},
				// An empty embed entry must not become a bogus qualified name.
				"Empty": {Name: pool.Get("Empty"), Embeds: []int{pool.Get("")}},
			}},
			"nil.go": nil,
		}},
	}

	facts := TypeFactsFor(meta)

	if !facts.Embeds("example.com/app.Handler", "other.io/pkg.Base") {
		t.Error("a cross-package embed was not seen; the name already carries its package and must not be re-qualified")
	}
	if facts.Embeds("example.com/app.Handler", "example.com/app.Base") {
		t.Error("a cross-package embed was re-qualified into the embedding type's own package")
	}
	if facts.Embeds("example.com/app.Empty", "example.com/app.Base") {
		t.Error("an empty embed entry produced a match")
	}
	if facts.Embeds("example.com/missing.Thing", "example.com/app.Base") {
		t.Error("a type in an unloaded package reported an embedding relation")
	}
	if facts.IsInterface("NoDotHere") {
		t.Error("an unqualified name was looked up as though it had a package")
	}
}
