// Copyright 2025 Ehab Terra
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
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// FrameworkDetector detects the web framework used in a project
type FrameworkDetector struct{}

// NewFrameworkDetector creates a new framework detector
func NewFrameworkDetector() *FrameworkDetector {
	return &FrameworkDetector{}
}

// Detect determines the primary web framework used in the given directory:
// the first framework import encountered in file-walk order (lexical, so
// deterministic). Kept as DetectAll's head for backwards compatibility.
func (d *FrameworkDetector) Detect(dir string) (string, error) {
	frameworks, err := d.DetectAll(dir)
	if err != nil {
		return "", err
	}
	return frameworks[0], nil
}

// DetectAll returns every recognised framework imported anywhere in the
// directory, in first-seen order (the head is what Detect historically
// returned and is used as the primary config). "net/http" appears only as
// the fallback when no framework import is found — its import is
// near-universal and carries no routing signal, so the stdlib surface is
// handled by the engine's always-on scoped merge instead.
func (d *FrameworkDetector) DetectAll(dir string) ([]string, error) {
	goFiles, err := CollectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	var frameworks []string
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			frameworks = append(frameworks, name)
		}
	}

	// ImportsOnly: parsing stops after the import block, which is all this
	// scan reads — a full parse of every file (the pre-DetectAll code at
	// least early-returned on the first hit) costs hundreds of ms on large
	// projects. The loop also stops once every known framework is seen; the
	// bound comes from the registry rather than a restated literal, which used
	// to abandon the walk at five and hide any sixth framework (issue #285).
	detectable := SourceDetectableFrameworks()
	fset := token.NewFileSet()
	for _, filePath := range goFiles {
		f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			continue // Skip files that can't be parsed
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			for _, fw := range detectable {
				if matchesAny(importPath, fw.SourcePatterns) {
					add(fw.Name)
					break
				}
			}
		}
		if len(frameworks) == len(detectable) {
			break
		}
	}

	if len(frameworks) == 0 {
		frameworks = append(frameworks, StdlibFramework)
	}
	return frameworks, nil
}

// matchesAny reports whether importPath contains any of the given substrings.
func matchesAny(importPath string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(importPath, p) {
			return true
		}
	}
	return false
}

// CollectGoFiles recursively collects all .go files from a directory
func CollectGoFiles(dir string) ([]string, error) {
	var goFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	return goFiles, err
}
