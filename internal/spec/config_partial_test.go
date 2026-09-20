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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeConfig writes a config file inside the test's own directory — a test
// must never dirty the working tree.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apispec.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestLoadAPISpecConfigOntoMerges pins that a config file is layered over the
// detected framework's defaults key by key, at every level of nesting
// (issue #524).
//
// A supplied file used to REPLACE the composed configuration, so one setting
// only `info:` carried no route patterns and the run documented zero paths
// while exiting 0. Replacing wholesale was wrong in a subtler way too: a file
// that did name `routePatterns` silently lost the response, parameter and
// security patterns it never mentioned.
//
// Emptying a part is still possible and is now said out loud, which is the
// distinction a struct cannot carry on its own.
func TestLoadAPISpecConfigOntoMerges(t *testing.T) {
	base := DefaultChiConfig()
	if len(base.Framework.RoutePatterns) == 0 || len(base.Framework.ResponsePatterns) == 0 {
		t.Fatal("the composed config lacks the patterns this test reasons about")
	}
	wantRoutes := len(base.Framework.RoutePatterns)
	wantResponses := len(base.Framework.ResponsePatterns)

	cases := []struct {
		name          string
		body          string
		routes        int
		responses     int
		wantTitle     string
		wantCtxRegexe string
		why           string
	}{
		{
			name:   "a config that never mentions framework",
			body:   "info:\n  title: Users API\n",
			routes: wantRoutes, responses: wantResponses, wantTitle: "Users API",
			why: "setting a title expresses no opinion about routing",
		},
		{
			name:   "an empty framework block",
			body:   "framework: {}\n",
			routes: wantRoutes, responses: wantResponses,
			why: "naming the key covers no part of it, so every part is inherited",
		},
		{
			name:   "a framework block that sets one unrelated part",
			body:   "framework:\n  requestContext:\n    typeRegexes: ['^myfw\\.Ctx$']\n",
			routes: wantRoutes, responses: wantResponses, wantCtxRegexe: `^myfw\.Ctx$`,
			why: "describing a request context must not cost the route patterns",
		},
		{
			name:   "a framework block that replaces one list",
			body:   "framework:\n  routePatterns:\n    - callRegex: ^Handle$\n",
			routes: 1, responses: wantResponses,
			why: "the list stated is the list used; the ones not stated are inherited",
		},
		{
			name:   "a part emptied on purpose",
			body:   "framework:\n  routePatterns: []\n",
			routes: 0, responses: wantResponses,
			why: "an explicit empty list is how a config says 'none', which omission cannot",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadAPISpecConfigOnto(writeConfig(t, tc.body), DefaultChiConfig())
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if got := len(cfg.Framework.RoutePatterns); got != tc.routes {
				t.Errorf("routePatterns = %d, want %d — %s", got, tc.routes, tc.why)
			}
			if got := len(cfg.Framework.ResponsePatterns); got != tc.responses {
				t.Errorf("responsePatterns = %d, want %d — a part the file never mentioned", got, tc.responses)
			}
			if tc.wantTitle != "" && cfg.Info.Title != tc.wantTitle {
				t.Errorf("info.title = %q, want %q", cfg.Info.Title, tc.wantTitle)
			}
			if tc.wantCtxRegexe != "" {
				got := cfg.Framework.RequestContext.TypeRegexes
				if len(got) != 1 || got[0] != tc.wantCtxRegexe {
					t.Errorf("requestContext.typeRegexes = %v, want [%q]", got, tc.wantCtxRegexe)
				}
			}
			// Whatever the file said, the framework's own defaults survive
			// where it said nothing.
			if cfg.Defaults.ResponseContentType == "" {
				t.Error("the framework's default response content type was lost")
			}
		})
	}
}

// TestLoadAPISpecConfigStandsAlone pins that the plain loader is unchanged: it
// parses a file on its own terms, which is what every caller outside the engine
// expects of it.
func TestLoadAPISpecConfigStandsAlone(t *testing.T) {
	cfg, err := LoadAPISpecConfig(writeConfig(t, "info:\n  title: Users API\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Framework.RoutePatterns) != 0 {
		t.Errorf("route patterns appeared from nowhere: %d", len(cfg.Framework.RoutePatterns))
	}
	if cfg.Info.Title != "Users API" {
		t.Errorf("info.title = %q", cfg.Info.Title)
	}
}

// TestAdoptFrameworkPatternsLeavesCodeBuiltConfigsAlone is the regression the
// first version of this fix caused, and it is the more dangerous direction.
//
// A config built in code has no file to inspect, so the "did the file declare
// framework?" question answers false for every one of them — including
// `NewGenerator(DefaultChiConfig())` with its patterns deliberately narrowed.
// Adopting there would replace the caller's scoped patterns with the detected
// defaults: the same silent loss, pointed the other way.
func TestAdoptFrameworkPatternsLeavesCodeBuiltConfigsAlone(t *testing.T) {
	scoped := DefaultChiConfig()
	for i := range scoped.Framework.RoutePatterns {
		scoped.Framework.RoutePatterns[i].CallerPkgPatterns = []string{`/internal/api$`}
	}

	scoped.AdoptFrameworkPatterns(DefaultChiConfig())

	for i, p := range scoped.Framework.RoutePatterns {
		if len(p.CallerPkgPatterns) == 0 {
			t.Fatalf("route pattern %d lost its caller scope; the code-built config was overwritten", i)
		}
	}
}

// TestAdoptFrameworkPatternsIsInert covers the arguments that must do nothing.
func TestAdoptFrameworkPatternsIsInert(t *testing.T) {
	var nilCfg *APISpecConfig
	nilCfg.AdoptFrameworkPatterns(DefaultChiConfig()) // must not panic

	cfg := &APISpecConfig{}
	cfg.AdoptFrameworkPatterns(nil)
	if len(cfg.Framework.RoutePatterns) != 0 {
		t.Error("patterns appeared from a nil composed config")
	}

	// An in-code config with no opinion at all is the one code-built case that
	// SHOULD adopt: it is the library equivalent of an empty file.
	empty := &APISpecConfig{}
	empty.AdoptFrameworkPatterns(DefaultChiConfig())
	if len(empty.Framework.RoutePatterns) == 0 {
		t.Error("an empty in-code config did not adopt the detected patterns")
	}
}

// TestHasFrameworkOpinionCoversEveryField is the drift guard.
//
// The rule is "a code-built config that says anything about its framework is
// taken at its word". The first version asked that of an enumerated list of
// pattern slices, which already missed HandlerInterfaceMethods, the
// RequestContext accessors and most of ResponseContext — so a config setting
// only one of those had its whole framework block replaced and the setting
// silently discarded.
//
// Rather than a longer list, this walks the struct: every field, including any
// added later, must on its own count as an opinion.
func TestHasFrameworkOpinionCoversEveryField(t *testing.T) {
	typ := reflect.TypeOf(FrameworkConfig{})
	if typ.NumField() == 0 {
		t.Fatal("FrameworkConfig has no fields; the guard proves nothing")
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			cfg := &APISpecConfig{}
			set := reflect.ValueOf(&cfg.Framework).Elem().Field(i)
			if !set.CanSet() {
				t.Skipf("%s is unexported", field.Name)
			}
			set.Set(nonZeroValue(t, field.Type))

			if !cfg.hasFrameworkOpinion() {
				t.Errorf("a config setting only %s is not read as an opinion, so AdoptFrameworkPatterns "+
					"would replace the whole framework block and discard it", field.Name)
			}
			// And the consequence, end to end.
			cfg.AdoptFrameworkPatterns(DefaultChiConfig())
			if !reflect.DeepEqual(reflect.ValueOf(cfg.Framework).Field(i).Interface(), set.Interface()) {
				t.Errorf("%s was overwritten by the detected framework's value", field.Name)
			}
		})
	}
}

// nonZeroValue builds a value of t that is distinguishable from the zero value,
// for the field walk above.
func nonZeroValue(t *testing.T, typ reflect.Type) reflect.Value {
	t.Helper()
	switch typ.Kind() {
	case reflect.Slice:
		return reflect.MakeSlice(typ, 1, 1)
	case reflect.Map:
		m := reflect.MakeMap(typ)
		m.SetMapIndex(reflect.New(typ.Key()).Elem(), reflect.New(typ.Elem()).Elem())
		return m
	case reflect.String:
		return reflect.ValueOf("x").Convert(typ)
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(typ)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(1)).Convert(typ)
	case reflect.Ptr:
		return reflect.New(typ.Elem())
	case reflect.Struct:
		// Set the struct's own first settable field, so the value differs from
		// the zero struct without this helper needing to know its shape.
		v := reflect.New(typ).Elem()
		for i := 0; i < typ.NumField(); i++ {
			if v.Field(i).CanSet() {
				v.Field(i).Set(nonZeroValue(t, typ.Field(i).Type))
				return v
			}
		}
		t.Fatalf("no settable field in %s", typ)
	}
	t.Fatalf("nonZeroValue: unhandled kind %s for %s", typ.Kind(), typ)
	return reflect.Value{}
}
