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
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// serializerPatternConfig is chi's default plus the bare serializer pattern a
// user reasonably writes — and which `--output-config` from a release predating
// the sink-anchored default would hand them.
func serializerPatternConfig() *intspec.APISpecConfig {
	cfg := intspec.DefaultChiConfig()
	cfg.Framework.ResponsePatterns = append(cfg.Framework.ResponsePatterns, intspec.ResponsePattern{
		CallRegex:   `^Marshal$`,
		TypeFromArg: true,
		Deref:       true,
	})
	return cfg
}

// TestTestdata_SerializerPatternGated pins that a serializer contributes a
// response only when its bytes reach the writer (issue #519).
//
// The decisive assertion is the last one: the document must be the SAME with
// and without the pattern. The remedy offered before was "delete the pattern",
// which is lossy and asks a consumer to track each release's defaults; the
// pattern can now stay and say nothing new.
func TestTestdata_SerializerPatternGated(t *testing.T) {
	withPattern := loadTestdata(t, "serializer_pattern_gated", serializerPatternConfig())
	noDanglingRefs(t, withPattern)

	t.Run("an outbound request body is not a response", func(t *testing.T) {
		op := opFor(withPattern.Paths["/widgets"], "POST")
		if op == nil {
			t.Fatalf("POST /widgets missing; have %v", mapPathKeys(withPattern.Paths))
		}
		for status, resp := range op.Responses {
			for _, mt := range resp.Content {
				if mt.Schema != nil && mt.Schema.Ref != "" &&
					hasSuffixName(mt.Schema.Ref, "Payload") {
					t.Errorf("POST /widgets %s documents Payload — that is the provider's REQUEST body, "+
						"reached only through http.NewRequest", status)
				}
			}
		}
		if _, ok := op.Responses["201"]; !ok {
			t.Errorf("POST /widgets lost its own 201; have %v", statusKeys(op))
		}
	})

	t.Run("a marshal written to the writer keeps its type", func(t *testing.T) {
		op := opFor(withPattern.Paths["/widgets/1"], "GET")
		if op == nil {
			t.Fatalf("GET /widgets/1 missing")
		}
		resp, ok := op.Responses["200"]
		if !ok {
			t.Fatalf("GET /widgets/1 has no 200; have %v", statusKeys(op))
		}
		mt, ok := resp.Content["application/json"]
		if !ok || mt.Schema == nil || !hasSuffixName(mt.Schema.Ref, "Widget") {
			t.Errorf("GET /widgets/1 200 = %+v, want the Widget it marshals and writes", mt.Schema)
		}
	})

	// The whole point: the pattern is now redundant rather than harmful.
	t.Run("the pattern changes nothing", func(t *testing.T) {
		without := loadTestdata(t, "serializer_pattern_gated", intspec.DefaultChiConfig())
		if len(without.Paths) != len(withPattern.Paths) {
			t.Fatalf("path count differs: %d without, %d with", len(without.Paths), len(withPattern.Paths))
		}
		for path, item := range without.Paths {
			for _, method := range []string{"GET", "POST"} {
				a, b := opFor(item, method), opFor(withPattern.Paths[path], method)
				if (a == nil) != (b == nil) {
					t.Errorf("%s %s: present in one document and not the other", method, path)
					continue
				}
				if a == nil {
					continue
				}
				if len(a.Responses) != len(b.Responses) {
					t.Errorf("%s %s: %v without the pattern, %v with it", method, path, statusKeys(a), statusKeys(b))
				}
			}
		}
	})
}

// hasSuffixName reports whether a $ref names a component ending in the given
// short type name.
func hasSuffixName(ref, name string) bool {
	return len(ref) >= len(name) && ref[len(ref)-len(name):] == name
}
