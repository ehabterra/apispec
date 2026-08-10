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

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryInvariants(t *testing.T) {
	names := map[string]bool{}
	keys := map[string]bool{}
	ranks := map[int]string{}
	var stdlib *Framework

	for i, fw := range Frameworks() {
		if fw.Name == "" || fw.DependencyKey == "" {
			t.Errorf("framework %d has an empty Name or DependencyKey: %+v", i, fw)
		}
		if names[fw.Name] {
			t.Errorf("duplicate framework name %q", fw.Name)
		}
		names[fw.Name] = true
		if keys[fw.DependencyKey] {
			t.Errorf("duplicate dependency key %q", fw.DependencyKey)
		}
		keys[fw.DependencyKey] = true
		// Ranks decide which framework wins for a package importing several;
		// a tie makes that winner depend on declaration order instead.
		if other, dup := ranks[fw.DetectionRank]; dup {
			t.Errorf("frameworks %q and %q share DetectionRank %d", other, fw.Name, fw.DetectionRank)
		}
		ranks[fw.DetectionRank] = fw.Name
		if len(fw.ImportPatterns) == 0 {
			t.Errorf("framework %q has no ImportPatterns, so dependency analysis can never classify it", fw.Name)
		}
		if fw.Name == StdlibFramework {
			f := fw
			stdlib = &f
		}
	}

	if stdlib == nil {
		t.Fatalf("registry has no %q entry; it is the detection fallback", StdlibFramework)
	}
	if stdlib.DependencyKey != StdlibDependencyKey {
		t.Errorf("stdlib dependency key = %q, want %q", stdlib.DependencyKey, StdlibDependencyKey)
	}
	if stdlib.SourceDetectable() {
		t.Error("net/http must not be source-detectable: it is imported by nearly every project and carries no routing signal")
	}
}

// TestStdlibRanksLast pins the one ordering fact detection depends on: the
// net/http pattern matches nearly every handler package, so any framework must
// outrank it or every package is classified as stdlib.
func TestStdlibRanksLast(t *testing.T) {
	ranked := FrameworksByDetectionRank()
	if len(ranked) == 0 {
		t.Fatal("registry is empty")
	}
	if last := ranked[len(ranked)-1]; last.Name != StdlibFramework {
		t.Errorf("last framework by detection rank = %q, want %q", last.Name, StdlibFramework)
	}
}

// TestDetectAllFindsEveryRegisteredFramework is the guard for issue #285: the
// source scan used to stop the file walk at a hardcoded `knownFrameworks = 5`,
// so a sixth framework was found only in projects that happened not to import
// five others. The table is the registry itself, so this widens automatically
// when a framework is added — which is exactly what the old literal did not.
func TestDetectAllFindsEveryRegisteredFramework(t *testing.T) {
	detectable := SourceDetectableFrameworks()
	if len(detectable) < 2 {
		t.Skip("fewer than two source-detectable frameworks registered")
	}

	dir := t.TempDir()
	// One file per framework, and the framework whose file sorts last is the
	// one an early exit would drop.
	for i, fw := range detectable {
		src := fmt.Sprintf("package main\n\nimport _ %q\n", fw.ImportPatterns[0])
		path := filepath.Join(dir, fmt.Sprintf("f%02d.go", i))
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got, err := NewFrameworkDetector().DetectAll(dir)
	if err != nil {
		t.Fatalf("DetectAll failed: %v", err)
	}

	found := map[string]bool{}
	for _, name := range got {
		found[name] = true
	}
	for _, fw := range detectable {
		if !found[fw.Name] {
			t.Errorf("DetectAll dropped %q (got %v) — the scan stopped before every registered framework was seen", fw.Name, got)
		}
	}
}

// TestDetectAllSurvivesANewFramework is the direct reproduction of issue #285:
// it registers one extra framework and asserts the scan still finds every one.
// Against the old `const knownFrameworks = 5` this fails — the walk stopped
// after five distinct hits, so the sixth was found only when the project
// happened not to import five others.
func TestDetectAllSurvivesANewFramework(t *testing.T) {
	restore := frameworks
	t.Cleanup(func() { frameworks = restore })
	frameworks = append(append([]Framework{}, restore...), Framework{
		Name:           "testfw",
		DependencyKey:  "testfw",
		SourcePatterns: []string{"example.com/testfw"},
		ImportPatterns: []string{"example.com/testfw"},
		DetectionRank:  stdlibDetectionRank - 1,
	})

	detectable := SourceDetectableFrameworks()
	dir := t.TempDir()
	for i, fw := range detectable {
		src := fmt.Sprintf("package main\n\nimport _ %q\n", fw.ImportPatterns[0])
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.go", i)), []byte(src), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	got, err := NewFrameworkDetector().DetectAll(dir)
	if err != nil {
		t.Fatalf("DetectAll failed: %v", err)
	}
	if len(got) != len(detectable) {
		t.Fatalf("DetectAll found %d of %d registered frameworks (%v) — the file walk exited early", len(got), len(detectable), got)
	}
}

// TestDetectAllMatchesEverySourcePattern covers the alternative import paths a
// framework can be imported under (versioned modules, companion packages):
// each pattern on its own must identify the framework.
func TestDetectAllMatchesEverySourcePattern(t *testing.T) {
	for _, fw := range SourceDetectableFrameworks() {
		for i, pattern := range fw.SourcePatterns {
			t.Run(fmt.Sprintf("%s/%s", fw.Name, pattern), func(t *testing.T) {
				dir := t.TempDir()
				// ImportsOnly parsing never resolves the import, so the pattern
				// itself stands in for whatever path a project imports it under.
				src := fmt.Sprintf("package main\n\nimport _ %q\n", pattern)
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("m%d.go", i)), []byte(src), 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				got, err := NewFrameworkDetector().Detect(dir)
				if err != nil {
					t.Fatalf("Detect failed: %v", err)
				}
				if got != fw.Name {
					t.Errorf("Detect = %q, want %q for pattern %q", got, fw.Name, pattern)
				}
			})
		}
	}
}

func TestConfigurableFrameworkNames(t *testing.T) {
	names := ConfigurableFrameworkNames()
	if len(names) == 0 {
		t.Fatal("no framework ships a default config")
	}
	// The UI offers this list, so the stdlib fallback must be selectable.
	var hasStdlib bool
	for _, n := range names {
		hasStdlib = hasStdlib || n == StdlibFramework
	}
	if !hasStdlib {
		t.Errorf("ConfigurableFrameworkNames = %v, missing %q", names, StdlibFramework)
	}
}

// TestFrameworksReturnsACopy guards the accessors: callers project and sort
// these slices (the dependency analyser sorts by rank), and a shared backing
// array would let one caller reorder the registry for everyone else.
func TestFrameworksReturnsACopy(t *testing.T) {
	first := Frameworks()
	if len(first) == 0 {
		t.Fatal("registry is empty")
	}
	original := first[0].Name
	first[0].Name = "mutated"
	if Frameworks()[0].Name != original {
		t.Errorf("mutating the returned slice changed the registry: got %q, want %q", Frameworks()[0].Name, original)
	}

	// The struct copy is shallow, so the pattern slices need cloning too —
	// otherwise a caller editing fw.ImportPatterns[0] rewrites the registry
	// for every later caller.
	for _, accessor := range []struct {
		name string
		get  func() []Framework
	}{
		{"Frameworks", Frameworks},
		{"SourceDetectableFrameworks", SourceDetectableFrameworks},
	} {
		got := accessor.get()
		if len(got) == 0 || len(got[0].ImportPatterns) == 0 {
			t.Fatalf("%s returned nothing to mutate", accessor.name)
		}
		name, want := got[0].Name, got[0].ImportPatterns[0]
		got[0].ImportPatterns[0] = "mutated"

		for _, fw := range Frameworks() {
			if fw.Name == name && fw.ImportPatterns[0] != want {
				t.Errorf("%s: mutating ImportPatterns changed the registry entry %q: got %q, want %q",
					accessor.name, name, fw.ImportPatterns[0], want)
			}
		}
	}
}

func TestCanonicalFrameworkName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantOK  bool
		comment string
	}{
		{"gin", "gin", true, "exact"},
		{"GIN", "gin", true, "upper"},
		{"Gin", "gin", true, "title"},
		{"NET/HTTP", StdlibFramework, true, "the stdlib name carries a slash"},
		{"house-router", "house-router", false, "unknown names pass through unchanged"},
		{"", "", false, "empty stays empty"},
	}
	for _, tt := range tests {
		got, ok := CanonicalFrameworkName(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("CanonicalFrameworkName(%q) = (%q, %v), want (%q, %v) — %s",
				tt.in, got, ok, tt.want, tt.wantOK, tt.comment)
		}
	}
}
