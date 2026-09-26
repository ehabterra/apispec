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
	"os"
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ConfigListMerge pins that a config adding one response pattern
// keeps every built-in one (issue #571).
//
// The fixture's apispec.yaml adds a pattern for a house Respond helper and
// says nothing else. The file is layered over the chi defaults exactly as the
// CLI layers --config over the detected framework. Before the fix, naming
// responsePatterns replaced the built-in list, so /items and /export — which
// only the built-ins document — fell to a bare `default:` object while exiting 0.
func TestTestdata_ConfigListMerge(t *testing.T) {
	dir := filepath.Join("..", "testdata", "config_list_merge")
	cfg, err := spec.LoadAPISpecConfigOnto(filepath.Join(dir, "apispec.yaml"), spec.DefaultChiConfig())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory: %v", err)
	}
	noDanglingRefs(t, out)

	cases := []struct {
		path, media, why string
	}{
		{"/items/{id}", "application/json", "the pattern the config adds"},
		{"/items", "application/json", "the built-in encoder pattern the config never mentions"},
		{"/export", "application/pdf", "the built-in raw write with a declared Content-Type"},
	}
	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("path %q missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		op := item.Get
		if op == nil {
			t.Errorf("%s: no GET operation", tc.path)
			continue
		}
		resp, ok := op.Responses["200"]
		if !ok {
			t.Errorf("GET %s: no 200 response (%s); responses %v", tc.path, tc.why, statusKeys(op))
			continue
		}
		if _, ok := resp.Content[tc.media]; !ok {
			t.Errorf("GET %s 200: no %s content (%s)", tc.path, tc.media, tc.why)
		}
	}
}

// TestTestdata_ConfigRoutePatternOutranksBuiltin pins that a route pattern a
// config adds wins an overlap with a MORE SPECIFIC built-in. Route matchers pick
// by specificity, not by position, so being first in the list was not enough:
// chi's receiver-scoped ^Get$ scored higher than this unscoped one, ran after
// it, and overwrote its extraction (review of #571).
//
// methodFromCall: false is what makes the winner visible. Under the user's
// pattern no verb is read from the call, so every route takes the POST default;
// under chi's it is GET.
func TestTestdata_ConfigRoutePatternOutranksBuiltin(t *testing.T) {
	dir := filepath.Join("..", "testdata", "config_list_merge")
	path := filepath.Join(t.TempDir(), "apispec.yaml")
	body := `framework:
  routePatterns:
    - callRegex: ^Get$
      pathFromArg: true
      handlerFromArg: true
      pathArgIndex: 0
      handlerArgIndex: 1
      methodFromCall: false
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := spec.LoadAPISpecConfigOnto(path, spec.DefaultChiConfig())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory: %v", err)
	}
	for _, p := range []string{"/items", "/items/{id}", "/export"} {
		item, ok := out.Paths[p]
		if !ok {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
			continue
		}
		if item.Get != nil || item.Post == nil {
			t.Errorf("%s: documented by the built-in chi pattern (GET), not by the config's (POST default)", p)
		}
	}
}
