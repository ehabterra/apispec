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

package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// TestConfigEditorCoversEveryField guards the UI's config editor against the
// drift it had accumulated: the editor lists the fields it can edit by hand, so
// every detection capability added to the config since (write-destination
// gating, map-key parameter names, mount router typing, entrypoints, …) was
// invisible in the UI while working fine on the CLI. A user could only reach
// those fields by switching to YAML mode, with nothing saying so.
//
// The check is deliberately shallow — it asserts the field NAME appears in the
// editor source, not that it is wired correctly — because its job is to make the
// next config addition fail loudly here rather than be quietly missing.
//
// Nested structures are exempt: duplicating their defaults in JavaScript would
// be a second source of truth for values that live in Go. They round-trip
// untouched through the editor (patterns are merged field-wise) and are edited in
// YAML mode.
func TestConfigEditorCoversEveryField(t *testing.T) {
	source, err := os.ReadFile("assets/js/config.js")
	if err != nil {
		t.Fatalf("read config editor: %v", err)
	}
	editor := string(source)

	// exempt lists fields the editor deliberately does not offer, with the reason.
	exempt := map[string]string{
		// Nested objects — see the doc comment.
		"methodExtraction": "nested MethodExtractionConfig; defaults live in Go",
		"bodyTransforms":   "nested BodyTransform list; edited in YAML mode",
	}

	// Every config struct whose fields the structured editor is expected to reach.
	structs := []any{
		spec.RoutePattern{},
		spec.RequestBodyPattern{},
		spec.ResponsePattern{},
		spec.ParamPattern{},
		spec.MountPattern{},
		spec.SecurityPattern{},
		spec.EntrypointPattern{},
		spec.RequestContextConfig{},
		spec.ResponseContextConfig{},
	}

	for _, s := range structs {
		typ := reflect.TypeOf(s)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				name := jsonFieldName(typ.Field(i))
				if name == "" {
					continue
				}
				if reason, ok := exempt[name]; ok {
					if strings.Contains(editor, `"`+name+`"`) {
						t.Errorf("%s.%s is listed as exempt (%s) but the editor does offer it — drop the exemption", typ.Name(), name, reason)
					}
					continue
				}
				if !strings.Contains(editor, `"`+name+`"`) {
					t.Errorf("%s.%s is not editable in the UI — add it to config.js (or to the exempt list here, with a reason)", typ.Name(), name)
				}
			}
		})
	}
}

// TestConfigEditorCoversEveryFrameworkSection is the same guard one level up: a
// whole new section of the framework config (entrypointPatterns was one) is
// easier to miss than a field.
func TestConfigEditorCoversEveryFrameworkSection(t *testing.T) {
	source, err := os.ReadFile("assets/js/config.js")
	if err != nil {
		t.Fatalf("read config editor: %v", err)
	}
	editor := string(source)

	for _, typ := range []reflect.Type{
		reflect.TypeOf(spec.FrameworkConfig{}),
		reflect.TypeOf(spec.APISpecConfig{}),
	} {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				name := jsonFieldName(typ.Field(i))
				if name == "" {
					continue
				}
				if !strings.Contains(editor, name) {
					t.Errorf("%s.%s has no editor section — add one to config.js", typ.Name(), name)
				}
			}
		})
	}
}

// jsonFieldName returns a field's JSON name, or "" for fields that are not
// serialised.
func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}
	return tag
}

// TestGenerateRequestOptionsReachTheUI is the same guard as the config editor's,
// one level out: an analysis option the server accepts but the UI never sends is
// invisible to a user, which is how --resolve-call-graph shipped in the first
// place (CLI only, discoverable through --help alone).
//
// It checks the option is wired at both ends — the request struct declares it and
// the browser sends it — not that it behaves correctly; that is the engine's
// tests. Its job is to fail when the next option is added on one side only.
func TestGenerateRequestOptionsReachTheUI(t *testing.T) {
	actions, err := os.ReadFile("assets/js/actions.js")
	if err != nil {
		t.Fatalf("read actions.js: %v", err)
	}
	store, err := os.ReadFile("assets/js/store.js")
	if err != nil {
		t.Fatalf("read store.js: %v", err)
	}

	// Analysis options the UI is expected to offer, by their JSON name.
	options := []string{"legacyTracker", "resolveCallGraph"}

	typ := reflect.TypeOf(GenerateRequest{})
	declared := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		if name := jsonFieldName(typ.Field(i)); name != "" {
			declared[name] = true
		}
	}

	for _, option := range options {
		if !declared[option] {
			t.Errorf("GenerateRequest has no %q field; the UI would send an option the server ignores", option)
		}
		if !strings.Contains(string(actions), option) {
			t.Errorf("%q is never sent by the browser — the server accepts it and no user can reach it", option)
		}
		if !strings.Contains(string(store), option) {
			t.Errorf("%q has no state in the store, so it cannot be toggled or remembered", option)
		}
	}
}
