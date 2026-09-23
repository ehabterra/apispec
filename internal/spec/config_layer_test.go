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
	"bytes"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLayerPutsUserEntriesFirst pins the order a layered list takes: the user's
// entries ahead of every built-in, so the first-match response, request and
// param matchers try the user's pattern first, and the built-ins intact behind
// it for the calls it does not claim (issue #571).
func TestLayerPutsUserEntriesFirst(t *testing.T) {
	builtins := DefaultChiConfig().Framework.ResponsePatterns
	cfg, err := LoadAPISpecConfigOnto(writeConfig(t, `
framework:
  responsePatterns:
    - callRegex: ^Respond$
      calleePkgPatterns: ["^example\\.com/app$"]
      typeFromArg: true
      typeArgIndex: 2
`), DefaultChiConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := cfg.Framework.ResponsePatterns
	if len(got) != len(builtins)+1 {
		t.Fatalf("responsePatterns = %d, want %d built-ins + 1", len(got), len(builtins))
	}
	if got[0].CallRegex != "^Respond$" {
		t.Errorf("first pattern = %q, want the user's ^Respond$", got[0].CallRegex)
	}
	if !reflect.DeepEqual(got[1:], builtins) {
		t.Error("the built-ins behind the user's entry were reordered or altered")
	}
}

// TestLayerExportRoundTripsToDefaults pins that a config exported with
// --output-config, loaded back over the same release's defaults, yields exactly
// those defaults: copies of built-ins are dropped rather than doubled or moved.
// Without that, the documented way of starting a config would reshuffle
// first-match order on every run.
func TestLayerExportRoundTripsToDefaults(t *testing.T) {
	data, err := yaml.Marshal(DefaultChiConfig())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "exported.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadAPISpecConfigOnto(path, DefaultChiConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if want := DefaultChiConfig(); !reflect.DeepEqual(cfg.Framework, want.Framework) {
		t.Error("an export of the defaults, loaded over the defaults, did not reproduce them")
	}
}

// TestLayerReachesNestedLists pins that the lists inside requestContext and
// responseContext are layered too. An empty responseContext.writerTypeRegexes
// silently disables streamed-body detection, so replacing it with one custom
// writer type was as costly as replacing a pattern list.
func TestLayerReachesNestedLists(t *testing.T) {
	builtins := DefaultChiConfig().Framework.ResponseContext.WriterTypeRegexes
	if len(builtins) == 0 {
		t.Fatal("chi ships no writer types; the test proves nothing")
	}
	cfg, err := LoadAPISpecConfigOnto(writeConfig(t, `
framework:
  responseContext:
    writerTypeRegexes: ['^example\.com/app\.Writer$']
`), DefaultChiConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := cfg.Framework.ResponseContext.WriterTypeRegexes
	if len(got) != len(builtins)+1 || got[0] != `^example\.com/app\.Writer$` {
		t.Errorf("writerTypeRegexes = %v, want the user's entry ahead of %v", got, builtins)
	}

	cfg, err = LoadAPISpecConfigOnto(writeConfig(t, `
framework:
  replaceDefaults: [responseContext.writerTypeRegexes]
  responseContext:
    writerTypeRegexes: ['^example\.com/app\.Writer$']
`), DefaultChiConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Framework.ResponseContext.WriterTypeRegexes; len(got) != 1 {
		t.Errorf("writerTypeRegexes = %v, want only the user's entry under replaceDefaults", got)
	}
}

// TestLayerRejectsUnknownReplaceName pins that a misspelt replaceDefaults entry
// fails the load. Warning would leave the list merged while its author believes
// it replaced — the same silent divergence the layering exists to end.
func TestLayerRejectsUnknownReplaceName(t *testing.T) {
	_, err := LoadAPISpecConfigOnto(writeConfig(t, `
framework:
  replaceDefaults: [responsePattern]
`), DefaultChiConfig())
	if err == nil {
		t.Fatal("an unknown replaceDefaults name loaded without error")
	}
	for _, want := range []string{"responsePattern", "responsePatterns", "responseContext.writerTypeRegexes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestLayerReportsShadowedBuiltin pins the one diagnostic a frozen export gets:
// an entry that differs from the built-in for the same call — the old variant
// of a pattern this release has since improved — keeps winning, and says so.
// An exact copy of a built-in says nothing, since it changes nothing.
func TestLayerReportsShadowedBuiltin(t *testing.T) {
	builtin := DefaultChiConfig().Framework.ResponsePatterns[0]

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	stale := builtin
	stale.CalleePkgPatterns = append([]string{`^example\.com/stale$`}, builtin.CalleePkgPatterns...)
	data, err := yaml.Marshal(&APISpecConfig{Framework: FrameworkConfig{
		ResponsePatterns: []ResponsePattern{builtin, stale},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "stale.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAPISpecConfigOnto(path, DefaultChiConfig()); err != nil {
		t.Fatalf("load: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "framework.responsePatterns[1]") {
		t.Errorf("the differing entry was not reported; log:\n%s", out)
	}
	if strings.Contains(out, "framework.responsePatterns[0]") {
		t.Errorf("an exact copy of a built-in was reported; log:\n%s", out)
	}
}

// TestLayerExternalTypesByName pins that externalTypes, which the gin and fiber
// presets populate, are layered by type name: adding one type keeps gin.H, and
// restating gin.H replaces the preset's entry rather than sitting behind it.
func TestLayerExternalTypesByName(t *testing.T) {
	cfg, err := LoadAPISpecConfigOnto(writeConfig(t, `
externalTypes:
  - name: example.com/app.ID
    openapiType: {type: string}
  - name: github.com/gin-gonic/gin.H
    openapiType: {type: object, description: mine}
`), DefaultGinConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var names []string
	for _, e := range cfg.ExternalTypes {
		names = append(names, e.Name)
	}
	if len(cfg.ExternalTypes) != 2 {
		t.Fatalf("externalTypes = %v, want the user's two with gin.H not doubled", names)
	}
	if h := cfg.ExternalTypes[1]; h.OpenAPIType == nil || h.OpenAPIType.Description != "mine" {
		t.Errorf("gin.H = %+v, want the user's restatement", h.OpenAPIType)
	}

	cfg, err = LoadAPISpecConfigOnto(writeConfig(t, `
externalTypes:
  - name: example.com/app.ID
    openapiType: {type: string}
`), DefaultGinConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ExternalTypes) != 2 || cfg.ExternalTypes[1].Name != "github.com/gin-gonic/gin.H" {
		t.Errorf("adding one external type lost the preset's gin.H: %+v", cfg.ExternalTypes)
	}
}

// TestLayerFlagsSpecificityChosenLists pins that the entries a file adds to the
// route, mount and security lists outrank every built-in. Those matchers pick
// the most specific pattern rather than the first, so position alone would let
// a scoped built-in beat an unscoped user pattern on the same call. Copies of
// built-ins stay unflagged, since they change nothing.
func TestLayerFlagsSpecificityChosenLists(t *testing.T) {
	base := DefaultChiConfig()
	builtinRoute, builtinMount := base.Framework.RoutePatterns[0], base.Framework.MountPatterns[0]
	data, err := yaml.Marshal(&APISpecConfig{Framework: FrameworkConfig{
		RoutePatterns:    []RoutePattern{{CallRegex: "^Get$"}, builtinRoute},
		MountPatterns:    []MountPattern{{CallRegex: "^Mount$"}, builtinMount},
		SecurityPatterns: []SecurityPattern{{CallRegex: "^Use$", Scope: "router"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "apispec.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadAPISpecConfigOnto(path, DefaultChiConfig())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fw := cfg.Framework

	flagged := func(name string, n int, flag func(int) bool) {
		t.Helper()
		for i := 0; i < n; i++ {
			if flag(i) != (i == 0) {
				t.Errorf("%s[%d].fromConfig = %v, want only the added entry flagged", name, i, flag(i))
			}
		}
	}
	flagged("routePatterns", len(fw.RoutePatterns), func(i int) bool { return fw.RoutePatterns[i].fromConfig })
	flagged("mountPatterns", len(fw.MountPatterns), func(i int) bool { return fw.MountPatterns[i].fromConfig })
	flagged("securityPatterns", len(fw.SecurityPatterns), func(i int) bool { return fw.SecurityPatterns[i].fromConfig })

	// The flagged entry is the LEAST specific of its list, and still ranks first.
	userRoute := NewRoutePatternMatcher(fw.RoutePatterns[0], cfg, nil)
	userMount := NewMountPatternMatcher(fw.MountPatterns[0], cfg, nil)
	userSec := NewSecurityPatternMatcher(fw.SecurityPatterns[0], cfg, nil)
	for i := 1; i < len(fw.RoutePatterns); i++ {
		if p := NewRoutePatternMatcher(fw.RoutePatterns[i], cfg, nil).GetPriority(); p >= userRoute.GetPriority() {
			t.Errorf("built-in route pattern %d priority %d >= the config's %d", i, p, userRoute.GetPriority())
		}
	}
	for i := 1; i < len(fw.MountPatterns); i++ {
		if p := NewMountPatternMatcher(fw.MountPatterns[i], cfg, nil).GetPriority(); p >= userMount.GetPriority() {
			t.Errorf("built-in mount pattern %d priority %d >= the config's %d", i, p, userMount.GetPriority())
		}
	}
	for i := 1; i < len(fw.SecurityPatterns); i++ {
		if p := NewSecurityPatternMatcher(fw.SecurityPatterns[i], cfg, nil).GetPriority(); p >= userSec.GetPriority() {
			t.Errorf("built-in security pattern %d priority %d >= the config's %d", i, p, userSec.GetPriority())
		}
	}
}

// TestLayerHelpersEdgeShapes covers the element and field shapes the shipped
// config happens not to contain today, so a field added later is still named
// the way yaml.v3 reads it and a list without callRegex never reports.
func TestLayerHelpersEdgeShapes(t *testing.T) {
	type probe struct {
		Tagged   string `yaml:"tagged,omitempty"`
		Untagged string
		Skipped  string `yaml:"-"`
		hidden   string //nolint:unused // exercises the unexported-field branch
	}
	typ := reflect.TypeOf(probe{})
	want := []string{"tagged", "untagged", "", ""}
	for i, w := range want {
		if got := yamlName(typ.Field(i)); got != w {
			t.Errorf("yamlName(%s) = %q, want %q", typ.Field(i).Name, got, w)
		}
	}

	if got := callRegexOf(reflect.ValueOf(ErrorSentinel{})); got != "" {
		t.Errorf("callRegexOf(ErrorSentinel) = %q, want none", got)
	}
	if got := callRegexOf(reflect.ValueOf("^Get$")); got != "" {
		t.Errorf("callRegexOf(string) = %q, want none", got)
	}
}
