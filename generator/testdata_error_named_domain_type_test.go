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

// TestTestdata_ErrorNamedDomainType pins that a type's NAME never decides which
// response body survives (issue #287).
//
// The fixture holds the two shapes a substring test gets backwards: an ordinary
// SLO domain type whose name contains "error" (ErrorBudgetReport, the success
// body), and the genuine RFC 7807 error body whose name contains nothing an
// "error" test can see (ProblemDetails) — plus MirrorConfig, where "error" is a
// substring with no word boundary near it.
//
// HONEST SCOPE: this fixture does NOT exercise preferResponseInfo, and it passed
// before the removal as well as after. Both statuses here resolve (200 and 400),
// so the two bodies land in different slots and never compete; the tie-break runs
// only in the "default" collapse, where every candidate has StatusCode < 0. A
// fixture that reaches it would have to make BOTH statuses unresolvable, at which
// point it pins the behaviour of a guess rather than of a fact.
//
// What it is, then, is a change-detector for the outcome that matters: these three
// types are documented on the operations that actually return them, and no future
// error-classification attempt may quietly demote a domain type again. The
// evidence for the removal itself is the unit test in extractor_sweep_test.go and
// the measurement recorded on preferResponseInfo — 10,347 reaches across three
// large real projects, zero decided by the name.
func TestTestdata_ErrorNamedDomainType(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "error_named_domain_type")

	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, tc := range []struct {
		path   string
		status string
		want   string
		why    string
	}{
		{"/budget", "200", "ErrorBudgetReport",
			"the success body; an error budget is a quantity, and demoting it drops the handler's real 200"},
		{"/budget", "400", "ProblemDetails",
			"the genuine error body, which no name test can recognise"},
		{"/mirror", "200", "MirrorConfig",
			"contains \"error\" only as a substring, with no word boundary near it"},
	} {
		t.Run(tc.path+"/"+tc.status, func(t *testing.T) {
			item, ok := out.Paths[tc.path]
			if !ok {
				t.Fatalf("path %s missing", tc.path)
			}
			op := opFor(item, "POST")
			if op == nil {
				t.Fatalf("POST %s missing", tc.path)
			}
			resp, ok := op.Responses[tc.status]
			if !ok {
				t.Fatalf("%s %s: no %s response", tc.path, "POST", tc.status)
			}
			ref := refOf(resp.Content)
			if !strings.HasSuffix(ref, "_"+tc.want) {
				t.Fatalf("%s %s documents %q, want %s — %s",
					tc.path, tc.status, ref, tc.want, tc.why)
			}
			// Naming the right type is only half of it: the $ref has to land on a
			// component that exists, with the fields the Go type declares.
			comp := componentByName(out, "_"+tc.want)
			if comp == nil {
				t.Fatalf("%s resolves to no component schema", ref)
			}
			if len(comp.Properties) == 0 {
				t.Errorf("component for %s has no properties — the type was named but never resolved", tc.want)
			}
		})
	}
}

// refOf returns the $ref of a JSON response body, or "" if it has none.
func refOf(content map[string]intspec.MediaType) string {
	mt, ok := content["application/json"]
	if !ok || mt.Schema == nil {
		return ""
	}
	return mt.Schema.Ref
}
