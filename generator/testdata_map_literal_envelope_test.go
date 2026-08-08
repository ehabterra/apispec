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

// TestTestdata_MapLiteralEnvelope pins that a response written as a map
// composite literal is documented as the object it is (issue #295).
//
// `map[string]any` is all the TYPE says, so mapping from the type alone can only
// produce `additionalProperties: {type: object}` and the payload's own component
// is never reached — a type-fidelity loss that an "untyped response" check does
// NOT catch, because the schema is non-empty, just useless.
//
// The cases are chosen so the fix cannot be a blanket "map literals become
// objects": /dynamic and /intkey must still produce exactly what they produced
// before, since additionalProperties is the right answer there.
func TestTestdata_MapLiteralEnvelope(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "map_literal_envelope")

	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	schemaOf := func(t *testing.T, path string) *intspec.Schema {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %s missing", path)
		}
		op := opFor(item, "POST")
		if op == nil {
			t.Fatalf("POST %s missing", path)
		}
		resp, ok := op.Responses["200"]
		if !ok {
			t.Fatalf("%s: no 200 response", path)
		}
		mt, ok := resp.Content["application/json"]
		if !ok || mt.Schema == nil {
			t.Fatalf("%s: no JSON schema", path)
		}
		return mt.Schema
	}

	t.Run("constant keys become properties", func(t *testing.T) {
		s := schemaOf(t, "/envelope")
		if s.AdditionalProperties != nil {
			t.Errorf("every key is a constant, so nothing is left for additionalProperties")
		}
		// The payload keeps its own component rather than collapsing to
		// {type: object} — the part a `schema: {}` check cannot see.
		codes := s.Properties["cost_codes"]
		if codes == nil || codes.Type != "array" || codes.Items == nil || !strings.HasSuffix(codes.Items.Ref, "_CostCode") {
			t.Errorf("cost_codes = %+v, want an array of $ref CostCode", codes)
		}
		// A struct value must $ref the SAME fully-qualified component every
		// other type in the document uses: reading the value's type raw gives
		// a same-package struct its bare name, which keys a second naming
		// convention into one spec.
		if meta := s.Properties["meta"]; meta == nil || !strings.HasSuffix(meta.Ref, "_Meta") {
			t.Errorf("meta = %+v, want a $ref to the qualified Meta component", meta)
		}
		// A literal value has no recorded type at all; it still has a Go type.
		if cur := s.Properties["cursor"]; cur == nil || cur.Type != "string" {
			t.Errorf("cursor = %+v, want {type: string}", cur)
		}
	})

	t.Run("an unreadable key leaves the rest possible", func(t *testing.T) {
		s := schemaOf(t, "/mixed")
		if codes := s.Properties["cost_codes"]; codes == nil || codes.Type != "array" {
			t.Errorf("the constant key must still be documented, got %+v", codes)
		}
		if s.AdditionalProperties == nil {
			t.Error("a computed key must stay possible, not be denied — documenting only the " +
				"known keys would claim the others cannot occur")
		}
	})

	// The two shapes additionalProperties is actually FOR. If these change, the
	// fix has become a blanket rule instead of reading the literal.
	t.Run("a runtime-built map stays additionalProperties", func(t *testing.T) {
		s := schemaOf(t, "/dynamic")
		if len(s.Properties) > 0 {
			t.Errorf("no literal to read, so no properties may be invented: %+v", s.Properties)
		}
		if s.AdditionalProperties == nil || !strings.HasSuffix(s.AdditionalProperties.Ref, "_CostCode") {
			t.Errorf("additionalProperties = %+v, want a $ref to CostCode", s.AdditionalProperties)
		}
	})

	t.Run("a non-string key is not an object", func(t *testing.T) {
		s := schemaOf(t, "/intkey")
		if len(s.Properties) > 0 {
			t.Errorf("map[int]string keys are not OpenAPI property names: %+v", s.Properties)
		}
	})

	t.Run("control: no envelope is unaffected", func(t *testing.T) {
		s := schemaOf(t, "/typed")
		if s.Type != "array" || s.Items == nil || !strings.HasSuffix(s.Items.Ref, "_CostCode") {
			t.Errorf("unwrapped payload = %+v, want an array of $ref CostCode", s)
		}
	})
}
