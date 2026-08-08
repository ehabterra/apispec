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
)

// TestTestdata_HTTPCatchallAnchor pins that the net/http response catch-all only
// matches calls the response writer actually reaches (issue #302).
//
// The preset matches any call named JSON/String/XML/YAML/ProtoBuf/Data/File/
// Redirect and documents its second argument as the response body. Unanchored,
// that included calls in any reached package that merely share a name — and
// APISpec shipped it that way, so this was a default-config defect, not a
// misconfiguration.
//
// The two assertions are a pair and neither is sufficient alone: the gate must
// drop the impostor AND keep the house renderer, since the cheapest way to pass
// the first is to stop matching helpers altogether.
func TestTestdata_HTTPCatchallAnchor(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "http_catchall_anchor")

	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	refsFor := func(t *testing.T, path string) string {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("%s missing", path)
		}
		op := opFor(item, "POST")
		if op == nil {
			t.Fatalf("POST %s missing", path)
		}
		var refs []string
		for _, resp := range op.Responses {
			for _, mt := range resp.Content {
				if mt.Schema != nil && mt.Schema.Ref != "" {
					refs = append(refs, mt.Schema.Ref)
				}
			}
		}
		return strings.Join(refs, " ")
	}

	// report.Data(ctx, payload) is named like a renderer and takes a payload
	// second, but never receives the writer — it cannot be this response.
	widget := refsFor(t, "/widget")
	if strings.Contains(widget, "_report_Payload") {
		t.Errorf("/widget responses %q include the analytics payload; a call that never "+
			"receives the response writer is not writing the response", widget)
	}
	if !strings.Contains(widget, "_Widget") {
		t.Errorf("/widget responses %q lost the real body", widget)
	}

	// The catch-all's own shape, handed the writer, must still match — otherwise
	// "anchoring" would just mean the catch-all stopped working.
	if gadget := refsFor(t, "/gadget"); !strings.Contains(gadget, "_Gadget") {
		t.Errorf("/gadget responses %q lost their body; a catch-all call that IS handed "+
			"the writer must still resolve", gadget)
	}
}
