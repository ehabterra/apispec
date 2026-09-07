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

	"github.com/ehabterra/apispec/internal/spec"
)

// naming.schemaNames: short must shorten every component AND keep the document
// resolvable — a rename that misses one $ref site produces a spec that cannot
// be consumed at all, which is worse than the long names it replaced (#298).
func TestTestdata_NamingShortSchemas(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Naming.SchemaNames = spec.NamingShort
	out := loadTestdata(t, "one_type_one_component", cfg)

	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for name := range out.Components.Schemas {
		if strings.Contains(name, "twoname") {
			t.Errorf("component %q still carries the module path", name)
		}
	}
	for _, want := range []string{"Issue", "Thing"} {
		if _, ok := out.Components.Schemas[want]; !ok {
			t.Errorf("want a component named %q; have %v", want, mapSchemaKeys(out.Components.Schemas))
		}
	}
}

// The default must not move: `full` is the documented default, and a project
// that says nothing about naming gets exactly what it got before.
func TestTestdata_NamingDefaultsToFull(t *testing.T) {
	out := loadTestdata(t, "one_type_one_component", spec.DefaultHTTPConfig())
	found := false
	for name := range out.Components.Schemas {
		if strings.Contains(name, "twoname_api") {
			found = true
		}
	}
	if !found {
		t.Errorf("default naming should keep fully-qualified components; have %v",
			mapSchemaKeys(out.Components.Schemas))
	}
}

// method-path ids are derived from the operation itself, so they carry no Go
// symbol and are unique by construction (a method and path pair is unique in
// OpenAPI).
func TestTestdata_NamingMethodPathOperationIDs(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Naming.OperationID = spec.NamingMethodPath
	out := loadTestdata(t, "one_type_one_component", cfg)

	seen := map[string]string{}
	for path, item := range out.Paths {
		item := item
		op := firstOperation(&item)
		if op == nil {
			continue
		}
		id := op.OperationID
		if id == "" {
			t.Errorf("%s: empty operationId", path)
			continue
		}
		if strings.Contains(id, "/") || strings.Contains(id, "twoname") {
			t.Errorf("%s: operationId %q still carries a Go symbol", path, id)
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q used by both %s and %s", id, prev, path)
		}
		seen[id] = path
	}
	if got, ok := seen["getDirect"]; !ok {
		t.Errorf("want an operationId %q derived from GET /direct; have %v", "getDirect", seen)
	} else if got != "/direct" {
		t.Errorf("getDirect mapped to %s", got)
	}
}

// receiver-method keeps the handler's own name and drops the import path.
func TestTestdata_NamingReceiverMethodOperationIDs(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Naming.OperationID = spec.NamingReceiverMethod
	out := loadTestdata(t, "one_type_one_component", cfg)

	for path, item := range out.Paths {
		item := item
		op := firstOperation(&item)
		if op == nil {
			continue
		}
		if strings.Contains(op.OperationID, "/") {
			t.Errorf("%s: operationId %q still carries an import path", path, op.OperationID)
		}
	}
}

// An unknown value keeps the default rather than failing the run or inventing
// a third behaviour.
func TestTestdata_NamingUnknownValueKeepsFull(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Naming.SchemaNames = "abbreviated"
	cfg.Naming.OperationID = "verb-noun"
	out := loadTestdata(t, "one_type_one_component", cfg)

	noDanglingRefs(t, out)
	found := false
	for name := range out.Components.Schemas {
		if strings.Contains(name, "twoname_api") {
			found = true
		}
	}
	if !found {
		t.Errorf("an unknown naming value should keep the fully-qualified default; have %v",
			mapSchemaKeys(out.Components.Schemas))
	}
}
