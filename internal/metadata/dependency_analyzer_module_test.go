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

package metadata

import (
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestProjectPackageClassificationByModulePath covers issue #282: the detector
// used to infer "is this ours" from import-path shape while go.mod had the
// answer. The inference was not merely redundant — for a domain-hosted module
// it produced no root at all, so every third-party import outside the small
// ExternalPrefixes allowlist was classified as a project package on the
// strength of containing two slashes.
func TestProjectPackageClassificationByModulePath(t *testing.T) {
	const module = "github.com/acme/billing"
	fd := NewFrameworkDetectorForModule(module)

	tests := []struct {
		path string
		want bool
		why  string
	}{
		{module, true, "the module root package itself"},
		{module + "/internal/handlers", true, "under the module"},
		{module + "/api", true, "under the module"},

		// The byte-prefix trap the issue names: billingv2 shares every byte of
		// billing but is a different module.
		{module + "v2", false, "a sibling module sharing a byte prefix"},
		{module + "v2/api", false, "under a sibling module sharing a byte prefix"},

		// Previously true, via `strings.Count(path, "/") >= 2`.
		{"github.com/other/lib", false, "unrelated third-party package"},
		{"github.com/acme/otherproject", false, "same org, different module"},
		{"gitlab.com/x/y/z", false, "unrelated, deeper"},
	}

	for _, tt := range tests {
		if got := fd.isProjectRelatedPackage(tt.path); got != tt.want {
			t.Errorf("isProjectRelatedPackage(%q) = %v, want %v — %s", tt.path, got, tt.want, tt.why)
		}
	}

	// Mock/test exclusion runs before the module check and still applies.
	if fd.isProjectRelatedPackage(module + "/internal/mocks") {
		t.Error("mock packages under the module should still be excluded")
	}
}

// TestProjectPackageClassificationWithoutModule pins the fallback: without a
// module path the detector infers, and the inference must at least be
// self-consistent — a domain-hosted root resolves instead of being discarded
// for containing a dot.
func TestProjectPackageClassificationWithoutModule(t *testing.T) {
	fd := NewFrameworkDetector()
	// Non-nil: the no-root path walks each package's syntax.
	fd.packages["github.com/acme/x/api"] = &packages.Package{PkgPath: "github.com/acme/x/api"}
	fd.packages["github.com/acme/x/app"] = &packages.Package{PkgPath: "github.com/acme/x/app"}

	if got := fd.detectProjectRoot(); got != "github.com/acme/x" {
		t.Fatalf("detectProjectRoot = %q, want github.com/acme/x", got)
	}
	if !fd.isProjectRelatedPackage("github.com/acme/x/internal/db") {
		t.Error("a package under the inferred root should be project-related")
	}
	// Segment-boundary check: the root must not swallow a sibling that merely
	// shares its bytes.
	if fd.isProjectRelatedPackage("github.com/acme/xother/api") {
		t.Error("github.com/acme/xother is a different project than github.com/acme/x")
	}
}

// TestModulePathBeatsInference proves the two disagree, so the wiring matters:
// the same path is external under the real module path and "project-related"
// under the heuristics.
func TestModulePathBeatsInference(t *testing.T) {
	const path = "github.com/unrelated/library"

	if NewFrameworkDetectorForModule("github.com/acme/billing").isProjectRelatedPackage(path) {
		t.Errorf("%q must be external when go.mod says the module is github.com/acme/billing", path)
	}
	// The fallback still claims it — `strings.Count(path, "/") >= 2` is the last
	// resort. Pinned deliberately: it documents why passing the module path is
	// what makes the classification correct, and flips this test if the
	// heuristic is ever tightened.
	if !NewFrameworkDetector().isProjectRelatedPackage(path) {
		t.Errorf("expected the no-module heuristic to still claim %q; if it no longer does, this test and the comment above are stale", path)
	}
}
