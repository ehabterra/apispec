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
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// oasSchemaFile is the official OpenAPI 3.1 document schema, vendored from
// https://spec.openapis.org/oas/3.1/schema/2026-08-03 so the test needs no
// network. It validates the document's structure — required fields, allowed
// keys, `x-` extensions only where the spec permits them — but not Schema
// Objects against the JSON Schema dialect.
const (
	oasSchemaFile = "testdata/oas-3.1-schema-2026-08-03.json"
	oasSchemaID   = "https://spec.openapis.org/oas/3.1/schema/2026-08-03"
)

// TestTestdata_ValidOpenAPIDocument validates every fixture's generated spec
// against the OpenAPI 3.1 document schema, in both serialisations the CLI
// writes.
//
// The per-fixture structural tests check what a spec says — routes, $refs,
// placeholders — and none of them checked that it is a valid document at all.
// JSON and YAML are each checked because they are produced by different
// encoders from different struct tags, and either can break alone: the YAML
// was valid while the JSON — the CLI's default — wrote every synthesized
// parameter's `x-warning` under a forbidden "Extensions" key, invalid in 75
// of 163 fixtures (issue #589).
//
// The schema is looser than the prose spec in places: it accepts an empty
// `license.name`, so TestInfoLicenseNeverEmittedWithoutName guards that one.
func TestTestdata_ValidOpenAPIDocument(t *testing.T) {
	schema := compileOASSchema(t)

	dirs, err := filepath.Glob(filepath.Join("..", "testdata", "*", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no fixtures found under ../testdata")
	}
	for _, modFile := range dirs {
		dir := filepath.Dir(modFile)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			out, err := NewGenerator(nil).GenerateFromDirectory(dir)
			if err != nil {
				t.Fatalf("GenerateFromDirectory: %v", err)
			}

			jsonDoc, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			validateOASDocument(t, schema, "json", jsonDoc)

			yamlDoc, err := yaml.Marshal(out)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			// The validator reads JSON values, so the YAML is decoded and
			// re-encoded: a mapping key YAML decodes as a non-string (an
			// unquoted `200:`) cannot round-trip and fails here, as it would
			// in any tool that reads the YAML.
			var decoded any
			if err := yaml.Unmarshal(yamlDoc, &decoded); err != nil {
				t.Fatalf("yaml.Unmarshal of the generated YAML: %v", err)
			}
			viaYAML, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("generated YAML is not representable as JSON: %v", err)
			}
			validateOASDocument(t, schema, "yaml", viaYAML)
		})
	}
}

func compileOASSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(oasSchemaFile)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse %s: %v", oasSchemaFile, err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(oasSchemaID, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile(oasSchemaID)
	if err != nil {
		t.Fatalf("compile OAS schema: %v", err)
	}
	return schema
}

func validateOASDocument(t *testing.T, schema *jsonschema.Schema, format string, doc []byte) {
	t.Helper()
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		t.Fatalf("%s: re-read generated document: %v", format, err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Errorf("%s output is not a valid OpenAPI 3.1 document:\n%v", format, err)
	}
}
