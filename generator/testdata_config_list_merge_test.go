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
