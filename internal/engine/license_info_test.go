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

package engine

import (
	"path/filepath"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestInfoLicenseNeverEmittedWithoutName pins that `info.license` is either
// complete or absent. `license.name` is REQUIRED, and the defaults used to
// emit `license: {name: ""}` on every run without --license-name, so every
// generated document failed OpenAPI schema validation (issue #589).
func TestInfoLicenseNeverEmittedWithoutName(t *testing.T) {
	cases := []struct {
		name     string
		cliName  string
		cliURL   string
		fromFile *intspec.License
		want     *intspec.License
	}{
		{name: "defaults omit it"},
		{name: "a URL alone cannot form a license", cliURL: "https://example.com/LICENSE"},
		{
			name:    "flags supply a complete license",
			cliName: "MIT",
			cliURL:  "https://opensource.org/licenses/MIT",
			want:    &intspec.License{Name: "MIT", URL: "https://opensource.org/licenses/MIT"},
		},
		{
			name:     "a config-file license without a name is dropped too",
			fromFile: &intspec.License{URL: "https://example.com/LICENSE"},
		},
		{
			name:     "a config-file license wins over the flags",
			cliName:  "MIT",
			fromFile: &intspec.License{Name: "Apache-2.0"},
			want:     &intspec.License{Name: "Apache-2.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultEngineConfig()
			cfg.InputDir = filepath.Join("..", "..", "testdata", "gin")
			cfg.LicenseName = tc.cliName
			cfg.LicenseURL = tc.cliURL
			if tc.fromFile != nil {
				apiCfg := intspec.DefaultGinConfig()
				apiCfg.Info.License = tc.fromFile
				cfg.APISpecConfig = apiCfg
			}

			out, err := NewEngine(cfg).GenerateOpenAPI()
			if err != nil {
				t.Fatalf("GenerateOpenAPI: %v", err)
			}
			got := out.Info.License
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("license = %+v, want it omitted", *got)
			case tc.want != nil && got == nil:
				t.Errorf("license omitted, want %+v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("license = %+v, want %+v", *got, *tc.want)
			}
		})
	}
}
