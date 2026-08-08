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

// TestUnanchoredResponsePatternsReportsBareCallNames pins the rule of issue
// #294: a response pattern that extracts a body type but is anchored to nothing
// matches a bare call NAME anywhere in the handler's call graph, so it documents
// whatever that call happens to carry — including the body a client marshals for
// an OUTBOUND request.
//
// The test is over the ANCHORS, not over serializer names. Which calls are
// dangerous is not knowable from a regex; whether a pattern has any way to tell
// a response from an unrelated call is.
func TestUnanchoredResponsePatternsReportsBareCallNames(t *testing.T) {
	bare := ResponsePattern{CallRegex: `^Marshal$`, TypeFromArg: true, Deref: true}

	t.Run("bare call name is reported", func(t *testing.T) {
		cfg := &APISpecConfig{Framework: FrameworkConfig{ResponsePatterns: []ResponsePattern{bare}}}
		got := cfg.UnanchoredResponsePatterns()
		if len(got) != 1 {
			t.Fatalf("got %v, want the one unanchored pattern", got)
		}
		// The message has to name which pattern, or a config with twenty of them
		// is no more actionable than no message at all.
		if !strings.Contains(got[0], "responsePatterns[0]") || !strings.Contains(got[0], "^Marshal$") {
			t.Errorf("report %q names neither the index nor the regex", got[0])
		}
	})

	// Each of these is a way to tell a response from an unrelated call, so each
	// must silence the report on its own.
	for name, anchor := range map[string]func(*ResponsePattern){
		"recvType":                   func(p *ResponsePattern) { p.RecvType = "net/http.ResponseWriter" },
		"recvTypeRegex":              func(p *ResponsePattern) { p.RecvTypeRegex = `^net/http\.ResponseWriter$` },
		"functionNameRegex":          func(p *ResponsePattern) { p.FunctionNameRegex = `.*Handler$` },
		"requireResponseDestination": func(p *ResponsePattern) { p.RequireResponseDestination = true },
		"callerPkgPatterns":          func(p *ResponsePattern) { p.CallerPkgPatterns = []string{"^example.com/api"} },
		"callerRecvTypePatterns":     func(p *ResponsePattern) { p.CallerRecvTypePatterns = []string{"^example.com/api"} },
		"calleePkgPatterns":          func(p *ResponsePattern) { p.CalleePkgPatterns = []string{"^encoding/json$"} },
		"calleeRecvTypePatterns":     func(p *ResponsePattern) { p.CalleeRecvTypePatterns = []string{"^example.com/api"} },
	} {
		t.Run("anchored by "+name, func(t *testing.T) {
			p := bare
			anchor(&p)
			cfg := &APISpecConfig{Framework: FrameworkConfig{ResponsePatterns: []ResponsePattern{p}}}
			if got := cfg.UnanchoredResponsePatterns(); len(got) != 0 {
				t.Errorf("%s anchors the pattern, but it was reported: %v", name, got)
			}
		})
	}

	// A pattern that extracts no body type cannot misattribute one, whatever it
	// matches — the status-only patterns (WriteHeader, Status) are exactly this.
	t.Run("no body type extracted", func(t *testing.T) {
		p := ResponsePattern{CallRegex: `^WriteHeader$`, StatusFromArg: true}
		cfg := &APISpecConfig{Framework: FrameworkConfig{ResponsePatterns: []ResponsePattern{p}}}
		if got := cfg.UnanchoredResponsePatterns(); len(got) != 0 {
			t.Errorf("a status-only pattern was reported: %v", got)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		if got := (&APISpecConfig{}).UnanchoredResponsePatterns(); len(got) != 0 {
			t.Errorf("got %v, want nothing", got)
		}
	})
}

// TestPresetsAnchorTheirResponsePatterns keeps the check honest in the direction
// that matters most: it must not become noise a user cannot act on. Every pattern
// APISpec itself ships is anchored, so a default run reports nothing.
//
// The net/http catch-all is the one exception, and it is a real one rather than a
// tolerated false positive — it matches JSON/String/Data/File/Redirect by bare
// name in any reached package (issue #302). It is asserted here as the CURRENT
// state so this test flips the moment it is fixed, instead of the exception
// quietly outliving the defect.
func TestPresetsAnchorTheirResponsePatterns(t *testing.T) {
	for name, cfg := range map[string]*APISpecConfig{
		"chi":   DefaultChiConfig(),
		"gin":   DefaultGinConfig(),
		"echo":  DefaultEchoConfig(),
		"fiber": DefaultFiberConfig(),
		"mux":   DefaultMuxConfig(),
	} {
		t.Run(name, func(t *testing.T) {
			if got := cfg.UnanchoredResponsePatterns(); len(got) != 0 {
				t.Errorf("%s preset ships an unanchored response pattern: %v", name, got)
			}
		})
	}

	t.Run("http (known gap, issue #302)", func(t *testing.T) {
		got := DefaultHTTPConfig().UnanchoredResponsePatterns()
		if len(got) != 1 {
			t.Fatalf("got %d unanchored patterns, want exactly the known net/http catch-all; "+
				"if #302 is fixed, delete this subtest and add http to the table above", len(got))
		}
		if !strings.Contains(got[0], "JSON|String|XML") {
			t.Errorf("the unanchored net/http pattern is %q, not the catch-all this exception covers", got[0])
		}
	})
}
