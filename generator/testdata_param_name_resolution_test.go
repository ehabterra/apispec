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
	"strings"
	"testing"
)

// TestTestdata_ParamNameResolution pins how a parameter's wire name is decided.
//
// The name is what a client sends, so it has to be statically known. When it was
// not, the Go expression was documented instead, and two shapes produced
// parameters no request could ever match — both seen on real projects:
//
//	name: github.com/labstack/echo.HeaderXRequestID   (constant from an
//	                                                   unanalysed package)
//	name: string                                      (the parameter's TYPE,
//	                                                   for a name passed through
//	                                                   a wrapper)
//
// One handler covers every case, so the rule cannot be half-applied.
func TestTestdata_ParamNameResolution(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "param_name_resolution", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	op := opFor(out.Paths["/items/upload"], "POST")
	if op == nil {
		t.Fatalf("POST /items/upload missing; have %v", mapPathKeys(out.Paths))
	}

	byName := map[string]string{}
	for _, p := range op.Parameters {
		byName[p.Name] = p.In
	}

	// A constant this project declares resolves to its VALUE: a client sends
	// "X-Tenant", never "HeaderTenant".
	if got := byName["X-Tenant"]; got != "header" {
		t.Errorf("X-Tenant: in=%q, want header — a project constant must resolve to its value; have %v", got, byName)
	}
	// A literal, unchanged.
	if got := byName["scope"]; got != "query" {
		t.Errorf("scope: in=%q, want query; have %v", got, byName)
	}

	// The constant from echo cannot be resolved (that package is never
	// analysed), so NO parameter may be emitted for it — not one named after the
	// Go expression.
	for name := range byName {
		if strings.Contains(name, "HeaderXRequestID") || strings.Contains(name, "echo.") {
			t.Errorf("parameter %q documents a Go expression, not a wire name", name)
		}
		if name == "string" {
			t.Errorf("parameter named %q documents a TYPE, not a wire name", name)
		}
	}
	if len(byName) != 2 {
		t.Errorf("expected exactly X-Tenant and scope, have %v", byName)
	}

	// The file part's name arrives through a wrapper parameter
	// (`readFile(c, "avatar")`), which is only knowable by tracing the parameter
	// back to the call site. Getting this wrong is what documented a file part
	// as "string".
	if op.RequestBody == nil {
		t.Fatal("POST /items/upload: multipart requestBody missing")
	}
	mt, ok := op.RequestBody.Content["multipart/form-data"]
	if !ok {
		t.Fatalf("multipart/form-data missing; have %v", contentTypes(op.RequestBody))
	}
	if mt.Schema == nil || mt.Schema.Properties["avatar"] == nil {
		t.Fatalf("file part 'avatar' missing; have %v", propNames(mt.Schema))
	}
	if got := mt.Schema.Properties["avatar"].Format; got != "binary" {
		t.Errorf("avatar format = %q, want binary", got)
	}
}
