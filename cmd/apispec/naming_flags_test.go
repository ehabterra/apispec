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
	"strings"
	"testing"
)

// The naming styles are reachable from the command line, and a misspelt one
// fails loudly instead of silently producing the default "full" names.
func TestNamingFlags(t *testing.T) {
	config, err := parseFlags([]string{"--operation-id", "method-path", "--schema-names", "short", "./api"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if config.OperationIDNaming != "method-path" || config.SchemaNaming != "short" {
		t.Errorf("got operationId %q, schemaNames %q", config.OperationIDNaming, config.SchemaNaming)
	}

	unset, err := parseFlags([]string{"./api"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if unset.OperationIDNaming != "" || unset.SchemaNaming != "" {
		t.Error("an unset flag must leave the config's naming in charge")
	}

	for _, args := range [][]string{
		{"--operation-id", "shortest", "./api"},
		{"--schema-names", "shrot", "./api"},
	} {
		if _, err := parseFlags(args); err == nil || !strings.Contains(err.Error(), "unknown style") {
			t.Errorf("parseFlags(%v) = %v, want an unknown-style error", args, err)
		}
	}
}
