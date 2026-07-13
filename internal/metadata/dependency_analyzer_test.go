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

package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestFrameworkDetector_Configure(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test default values
	if fd.config.IncludeExternalPackages != false {
		t.Error("Expected default IncludeExternalPackages to be false")
	}
	// MaxImportDepth has a default value in the configuration

	// Test configuration
	fd.Configure(true, 5)

	if fd.config.IncludeExternalPackages != true {
		t.Error("Expected IncludeExternalPackages to be true after Configure")
	}
	if fd.config.MaxImportDepth != 5 {
		t.Error("Expected MaxImportDepth to be 5 after Configure")
	}
}

func TestFrameworkDetector_DisableFramework(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test disabling a framework
	fd.DisableFramework("http")

	if fd.config.DisabledFrameworks == nil {
		t.Error("Expected DisabledFrameworks map to be initialized")
	}

	if !fd.config.DisabledFrameworks["http"] {
		t.Error("Expected http framework to be disabled")
	}

	// Test disabling multiple frameworks
	fd.DisableFramework("chi")

	if !fd.config.DisabledFrameworks["chi"] {
		t.Error("Expected chi framework to be disabled")
	}
}

func TestNewFrameworkDetector(t *testing.T) {
	fd := NewFrameworkDetector()

	if fd == nil {
		t.Error("Expected non-nil FrameworkDetector")
		return
	}

	if fd.packages == nil {
		t.Error("Expected packages map to be initialized")
		return
	}

	if fd.dependencyGraph == nil {
		t.Error("Expected dependencyGraph map to be initialized")
		return
	}

	if fd.reverseDependencyGraph == nil {
		t.Error("Expected reverseDependencyGraph map to be initialized")
		return
	}
}

func TestNewFrameworkDetectorWithConfig(t *testing.T) {
	config := FrameworkDetectorConfig{
		IncludeExternalPackages: true,
		MaxImportDepth:          5,
	}

	fd := NewFrameworkDetectorWithConfig(config)

	if fd == nil {
		t.Error("Expected non-nil FrameworkDetector")
		return
	}

	if fd.config.IncludeExternalPackages != true {
		t.Error("Expected IncludeExternalPackages to be true")
	}

	if fd.config.MaxImportDepth != 5 {
		t.Error("Expected MaxImportDepth to be 5")
	}
}

func TestDefaultFrameworkDetectorConfig(t *testing.T) {
	config := DefaultFrameworkDetectorConfig()

	if config.IncludeExternalPackages != false {
		t.Error("Expected default IncludeExternalPackages to be false")
	}

	// MaxImportDepth has a default value in the configuration
	if config.MaxImportDepth < 0 {
		t.Error("Expected default MaxImportDepth to be non-negative")
	}

	if config.FrameworkPatterns == nil {
		t.Error("Expected FrameworkPatterns to be initialized")
	}

	if config.ExternalPrefixes == nil {
		t.Error("Expected ExternalPrefixes to be initialized")
	}

	if config.ProjectPatterns == nil {
		t.Error("Expected ProjectPatterns to be initialized")
	}

	if config.TestMockPatterns == nil {
		t.Error("Expected TestMockPatterns to be initialized")
	}

	if config.DisabledFrameworks == nil {
		t.Error("Expected DisabledFrameworks to be initialized")
	}
}

func TestFrameworkDetector_PureHelpers(t *testing.T) {
	fd := NewFrameworkDetector()
	if got := fd.findCommonPrefix("github.com/a/b", "github.com/a/c"); got != "github.com/a/" {
		t.Errorf("findCommonPrefix = %q", got)
	}
	if !fd.contains([]string{"x", "y"}, "y") || fd.contains([]string{"x"}, "z") {
		t.Error("contains wrong")
	}
	if !fd.isTestMockPackage("github.com/x/mocks") {
		t.Error("mocks should be a test/mock package")
	}
	if fd.isTestMockPackage("github.com/x/handlers") {
		t.Error("handlers is not a mock package")
	}
	if fd.isProjectRelatedPackage("github.com/gin-gonic/gin") {
		t.Error("gin is external, not project-related")
	}
}

func TestFrameworkDetector_DetectAndAnalyze(t *testing.T) {
	fset := token.NewFileSet()
	mainFile, err := parser.ParseFile(fset, "main.go", `package main
import "github.com/gin-gonic/gin"
func main() { _ = gin.New() }`, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	ginFile, err := parser.ParseFile(fset, "gin.go", `package gin`, 0)
	if err != nil {
		t.Fatalf("parse gin: %v", err)
	}

	ginPkg := &packages.Package{PkgPath: "github.com/gin-gonic/gin", Name: "gin", Syntax: []*ast.File{ginFile}}
	appPkg := &packages.Package{
		PkgPath: "example.com/app", Name: "main", Syntax: []*ast.File{mainFile},
		Imports: map[string]*packages.Package{"github.com/gin-gonic/gin": ginPkg},
	}

	fd := NewFrameworkDetector()
	if ft := fd.detectFrameworkType(appPkg); ft != "gin" {
		t.Errorf("detectFrameworkType(app) = %q, want gin", ft)
	}
	if ft := fd.detectFrameworkType(ginPkg); ft != "" {
		t.Errorf("detectFrameworkType(gin, no imports) = %q, want empty", ft)
	}

	list, err := fd.AnalyzeFrameworkDependencies(
		[]*packages.Package{appPkg, ginPkg},
		map[string]map[string]*ast.File{},
		map[*ast.File]*types.Info{},
		fset,
	)
	if err != nil {
		t.Fatalf("AnalyzeFrameworkDependencies: %v", err)
	}
	if list == nil || list.TotalPackages == 0 {
		t.Fatalf("expected framework packages, got %+v", list)
	}
	if list.TotalPackages != list.DirectPackages+list.IndirectPackages {
		t.Errorf("counts inconsistent: %+v", list)
	}
	_ = list.GetFrameworkPackages()
	_ = list.GetImportedPackages()
	_ = list.GetDirectPackages()
	_ = list.GetIndirectPackages()
	list.PrintDependencyList()
}
