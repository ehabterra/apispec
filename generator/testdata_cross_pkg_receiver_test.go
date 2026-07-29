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

// responseSchemaName returns the component a response names, whether directly or
// as the element type of an array — two of these handlers return a slice.
func responseSchemaName(content map[string]intspec.MediaType) string {
	for _, mt := range content {
		if mt.Schema == nil {
			continue
		}
		if mt.Schema.Ref != "" {
			return mt.Schema.Ref
		}
		if mt.Schema.Items != nil && mt.Schema.Items.Ref != "" {
			return mt.Schema.Items.Ref
		}
	}
	return ""
}

// TestTestdata_CrossPkgReceiver is regression coverage for the SPEC over the two
// shapes of issue #249. It is not the change-detector for that issue: the
// responses here resolve from the handler's return value, so they were already
// correct while the call graph underneath was wrong — see
// TestCrossPkgReceiverIsRecordedWithItsReceiver for the assertion that fails
// without the fix.
//
//	/subjects  a method on a service held in a struct field of another package
//	           — the control: this shape always worked
//	/aliased   the same call through a field typed by a LOCAL ALIAS of that
//	           service (`type curriculumAlias = curriculum.Service`), which is how
//	           a package avoids naming another at the type level. A *types.Alias
//	           matched no arm of the receiver type switch, so the call was
//	           recorded as a function of the caller's package with no receiver
//	           at all
//	/asset     a call through a field whose type is an INLINE interface, whose
//	           receiver was recorded as the literal string "interface" — a name
//	           nothing can look up
func TestTestdata_CrossPkgReceiver(t *testing.T) {
	out := loadTestdata(t, "cross_pkg_receiver", nil)

	cases := []struct {
		path   string
		schema string
		why    string
	}{
		{
			path:   "/subjects",
			schema: "curriculum_Subject",
			why:    "the control — a plain cross-package method on a struct field",
		},
		{
			path:   "/aliased",
			schema: "curriculum_Subject",
			why:    "reached through a local alias; the response must resolve to the type the ALIASED service returns, not be lost with the receiver",
		},
		{
			path:   "/asset",
			schema: "storage_Asset",
			why:    "reached through an inline interface; the concrete type is what the handler encodes",
		},
	}

	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			item, ok := out.Paths[tt.path]
			if !ok {
				t.Fatalf("%s missing; have %v", tt.path, mapPathKeys(out.Paths))
			}
			op := opFor(item, "GET")
			if op == nil {
				t.Fatalf("GET %s missing", tt.path)
			}
			resp, ok := op.Responses["200"]
			if !ok {
				t.Fatalf("GET %s: no 200; have %v — %s", tt.path, keysOf(op.Responses), tt.why)
			}
			ref := responseSchemaName(resp.Content)
			if !strings.Contains(ref, tt.schema) {
				t.Errorf("GET %s: response schema %q does not name %s — %s", tt.path, ref, tt.schema, tt.why)
			}
		})
	}
}

// TestCrossPkgReceiverIsRecordedWithItsReceiver is the change-detector for
// issue #249: it asserts on the recorded call graph, which is where the defect
// was.
//
// Losing the receiver is not cosmetic. It is what a pattern scopes on, so the
// call becomes unmatchable; and because the callee was then attributed to the
// CALLING package, the graph named a function that exists nowhere — which also
// made the call unjoinable to a resolved call graph.
func TestCrossPkgReceiverIsRecordedWithItsReceiver(t *testing.T) {
	// The fix this asserts is currently gated off (calleeSelectionEnabled): it is
	// correct and unaffordable, taking a 100k-node gitea run from 2m06 to over ten
	// minutes because it makes far more code reachable than the node budget can
	// fund. Kept as a change-detector so the day the budget stops counting keys
	// (#247) and the gate is opened, this passes and proves it.
	t.Skip("blocked on #247: the fix is gated off until expansion is bounded by what the spec asks for")

	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "cross_pkg_receiver")

	meta, err := engine.NewEngine(cfg).GenerateMetadataOnly()
	if err != nil {
		t.Fatalf("GenerateMetadataOnly: %v", err)
	}

	var subjectCalls []string
	var describeCalls []string
	for i := range meta.CallGraph {
		callee := meta.CallGraph[i].Callee.BaseID()
		switch {
		case strings.HasSuffix(callee, ".ListSubjects"):
			subjectCalls = append(subjectCalls, callee)
		case strings.HasSuffix(callee, ".Describe"):
			describeCalls = append(describeCalls, callee)
		}
	}

	// Two calls to ListSubjects: one through the concrete field, one through a
	// local alias of the same type. Both are the same method, so both must be
	// recorded against the type that declares it.
	const want = "example.com/recv/internal/curriculum.Service.ListSubjects"
	if len(subjectCalls) != 2 {
		t.Fatalf("recorded %d ListSubjects calls, want 2: %v", len(subjectCalls), subjectCalls)
	}
	for _, got := range subjectCalls {
		if got != want {
			t.Errorf("ListSubjects recorded as %q, want %q — a local alias of another package's type must not\n"+
				"re-home the method in the calling package, or no receiver-scoped pattern can match it", got, want)
		}
	}

	// An unnamed interface has no name to record, so the receiver must be empty
	// rather than the literal string "interface", which reads like a type name
	// and is not one.
	for _, got := range describeCalls {
		if strings.Contains(got, ".interface.") {
			t.Errorf("Describe recorded as %q — \"interface\" is not a type name and nothing can look it up", got)
		}
	}
	if len(describeCalls) == 0 {
		t.Error("no Describe call recorded at all; the inline-interface shape is not being covered")
	}
}
