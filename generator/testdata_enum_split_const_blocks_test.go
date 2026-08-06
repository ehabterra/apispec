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

package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_EnumSplitConstBlocks pins that a type's enum is ALL of its
// constants, however they are arranged in source.
//
// Enum detection used to document the single largest `const (...)` block and
// drop the rest. Any enum big enough to be worth writing down is grouped by
// meaning — creation triggers here, terminal transitions there, each under its
// own comment — so this hit real types hard: a 32-value type was documented with
// 6 values, and clients validating against it would reject 26 legitimate ones.
//
// The failure also MOVED. The winner was the biggest block, ties to the earliest,
// so appending a constant to a tied block flipped the whole enum to a different
// block's values — spec drift with no source change that explains it, and no
// signal that anything was dropped either time.
func TestTestdata_EnumSplitConstBlocks(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "enum_split_const_blocks")

	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	schema := findSchemaBySuffix(t, out, "_Candidate")

	// Reason's constants live in three blocks of 3, 2 and 3 — two of them tied
	// for largest, which is what made the old answer flip.
	assertEnum(t, schema, "reason", []string{
		"deadline_passed", "hw_completed", "lesson_completed", "over_maximum",
		"quiz_announced", "quiz_boundary", "unenrolled", "within_target",
	})

	// Kind is the control: one block, and unioning must not pull in Reason's
	// values just because both are string-underlying (issue #229).
	assertEnum(t, schema, "kind", []string{"exercise", "quiz"})
}

// findSchemaBySuffix returns the one component schema whose name ends in suffix.
func findSchemaBySuffix(t *testing.T, out *intspec.OpenAPISpec, suffix string) *intspec.Schema {
	t.Helper()
	var found *intspec.Schema
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, suffix) {
			if found != nil {
				t.Fatalf("more than one component ends in %q", suffix)
			}
			found = s
		}
	}
	if found == nil {
		t.Fatalf("no component schema ends in %q", suffix)
	}
	return found
}

// assertEnum compares a property's enum against want, as sorted strings.
func assertEnum(t *testing.T, schema *intspec.Schema, property string, want []string) {
	t.Helper()
	prop, ok := schema.Properties[property]
	if !ok {
		t.Fatalf("property %q missing", property)
	}
	got := make([]string, 0, len(prop.Enum))
	for _, v := range prop.Enum {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("property %q enum holds a non-string %T", property, v)
		}
		got = append(got, s)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("property %q enum =\n  %v\nwant\n  %v\n"+
			"every constant declared with the type belongs to it; a `const (...)` block is "+
			"source arrangement, not membership", property, got, want)
	}
}
