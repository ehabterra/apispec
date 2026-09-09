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
	"strings"
	"testing"
)

// These iterate the production frameworkConfigs map rather than a list of
// their own, so a framework added later is covered without touching them
// (golden rule #5).

// Every encoder in the ecosystem spells the call `Encode`, and
// responseMatcherIndex takes the FIRST matcher that accepts an edge rather than
// the highest-priority one. So an xml encoder pattern only ever wins while it
// precedes the JSON one; placed after it, it is unreachable and
// xml.NewEncoder(w).Encode(v) goes on being documented as JSON (issue #354).
//
// This pins the ORDER, which is the part a later edit can silently break — the
// fixtures would keep passing on net/http while chi, mux, echo and fiber
// regressed.
func TestNonJSONEncodersPrecedeTheJSONEncoder(t *testing.T) {
	for name, newCfg := range frameworkConfigs {
		cfg := newCfg()
		xmlAt, jsonAt := -1, -1
		for i, p := range cfg.Framework.ResponsePatterns {
			if p.CallRegex != `^Encode$` {
				continue
			}
			switch p.DefaultContentType {
			case contentTypeXML:
				if xmlAt < 0 {
					xmlAt = i
				}
			case "":
				if jsonAt < 0 {
					jsonAt = i
				}
			}
		}
		if xmlAt < 0 {
			t.Errorf("%s: no xml Encode response pattern — xml.NewEncoder(w).Encode is either "+
				"undocumented or documented as JSON", name)
			continue
		}
		if jsonAt < 0 {
			t.Errorf("%s: no JSON Encode response pattern", name)
			continue
		}
		if xmlAt > jsonAt {
			t.Errorf("%s: the xml Encode pattern is at %d, behind the JSON one at %d — "+
				"the first matcher wins, so it can never match", name, xmlAt, jsonAt)
		}
	}
}

// A renderer that names its format writes that format. The catch-all matched
// them all and let every one fall back to application/json.
func TestRenderersCarryTheMediaTypeTheyWrite(t *testing.T) {
	want := map[string]string{
		"XML":      contentTypeXML,
		"YAML":     contentTypeYAML,
		"String":   contentTypeText,
		"HTML":     contentTypeHTML,
		"ProtoBuf": contentTypeProtobuf,
	}
	// Only the configs that carry the renderer catch-all: fiber names its
	// renderers individually (c.JSON / c.SendString), chi and mux have none.
	for _, name := range []string{"net/http", "gin", "echo"} {
		cfg := DefaultConfigForFramework(name)
		got := map[string]string{}
		for _, p := range cfg.Framework.ResponsePatterns {
			for call := range want {
				if p.CallRegex == `^(?i)`+call+`$` {
					got[call] = p.DefaultContentType
				}
			}
			if strings.Contains(p.CallRegex, "XML|") {
				t.Errorf("%s: the renderer catch-all %q is back — every renderer under it "+
					"falls back to application/json", name, p.CallRegex)
			}
		}
		for call, ct := range want {
			if got[call] != ct {
				t.Errorf("%s: renderer %s has content type %q, want %q", name, call, got[call], ct)
			}
		}
	}
}

// The renderers whose media type is NOT knowable from the name keep the shared
// pattern and the config default rather than a guess (golden rule #7): Data
// takes its content type as an argument, File depends on the file, Redirect
// writes no body. JSON is left to Defaults.ResponseContentType so a project
// serving a vendor JSON type keeps saying so.
func TestUnknowableRenderersKeepTheConfigDefault(t *testing.T) {
	for _, name := range []string{"net/http", "gin", "echo"} {
		cfg := DefaultConfigForFramework(name)
		found := false
		for _, p := range cfg.Framework.ResponsePatterns {
			if p.CallRegex != rendererCatchAllCalls {
				continue
			}
			found = true
			if p.DefaultContentType != "" {
				t.Errorf("%s: the catch-all for JSON/Data/File/Redirect claims %q — "+
					"none of those names says what is written", name, p.DefaultContentType)
			}
		}
		if !found {
			t.Errorf("%s: no catch-all pattern for the renderers whose media type is not "+
				"derivable from the call", name)
		}
	}
}

// fiber's c.SendString writes a string body, which fiber sends as text/plain.
func TestFiberSendStringIsPlainText(t *testing.T) {
	cfg := DefaultFiberConfig()
	for _, p := range cfg.Framework.ResponsePatterns {
		if p.CallRegex == `^SendString$` {
			if p.DefaultContentType != contentTypeText {
				t.Errorf("SendString content type = %q, want %q", p.DefaultContentType, contentTypeText)
			}
			return
		}
	}
	t.Error("fiber has no SendString response pattern")
}
