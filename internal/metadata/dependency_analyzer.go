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
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/ehabterra/apispec/internal/core"
	"golang.org/x/tools/go/packages"
)

// FrameworkDependency represents a framework dependency
type FrameworkDependency struct {
	PackagePath   string                 `yaml:"package_path"`
	FrameworkType string                 `yaml:"framework_type"`
	IsDirect      bool                   `yaml:"is_direct"`
	Files         []string               `yaml:"files"`
	Functions     []string               `yaml:"functions"`
	Types         []string               `yaml:"types"`
	Metadata      map[string]interface{} `yaml:"metadata,omitempty"`
}

// FrameworkDependencyList represents a flat list of all framework-related packages
type FrameworkDependencyList struct {
	AllPackages      []*FrameworkDependency `yaml:"all_packages"`
	FrameworkTypes   map[string][]string    `yaml:"framework_types"`
	TotalPackages    int                    `yaml:"total_packages"`
	DirectPackages   int                    `yaml:"direct_packages"`
	IndirectPackages int                    `yaml:"indirect_packages"`
}

// FrameworkDetectorConfig holds configuration for framework detection.
// The framework patterns and external prefixes are projections of the registry
// in internal/core; the project/test/mock patterns below are heuristics that
// only apply when no module path is known (see FrameworkDetector.modulePath).
type FrameworkDetectorConfig struct {
	// FrameworkPatterns maps framework types to their import patterns.
	// Example: "gin" -> ["github.com/gin-gonic/gin", "github.com/gin-contrib/"]
	FrameworkPatterns map[string][]string

	// ExternalPrefixes are package prefixes that should be excluded as external dependencies.
	// Example: ["github.com/gin-gonic/gin", "golang.org/x/"]
	ExternalPrefixes []string

	// ProjectPatterns are patterns used for fallback project package detection.
	// Example: ["/models/", "/handlers/", "/services/"]
	ProjectPatterns []string

	// TestMockPatterns are patterns used to identify and exclude test/mock packages.
	// Example: ["/mock/", "/test/", "_mock", "_test"]
	TestMockPatterns []string

	// IncludeExternalPackages determines whether to include external packages in analysis.
	IncludeExternalPackages bool

	// MaxImportDepth controls the maximum depth for recursive import analysis.
	MaxImportDepth int

	// DisabledFrameworks contains framework types that should be skipped during detection.
	DisabledFrameworks map[string]bool
}

// FrameworkDetector detects framework dependencies using configurable patterns.
// It analyzes Go packages to identify framework usage and related dependencies,
// and classifies each import as belonging to the project or not.
type FrameworkDetector struct {
	config FrameworkDetectorConfig
	// modulePath is the module path from go.mod — the authoritative answer to
	// "is this package ours". Empty when the caller has no module (a bare
	// directory, or a test constructing the detector directly), which is the
	// only case the path heuristics below run in.
	modulePath string
	// Package analysis results from go/packages
	packages map[string]*packages.Package
	// Dependency graph: package -> its dependencies
	dependencyGraph map[string][]string
	// Reverse dependency graph: package -> packages that depend on it
	reverseDependencyGraph map[string][]string
}

// NewFrameworkDetector creates a new framework detector with default configuration
func NewFrameworkDetector() *FrameworkDetector {
	return NewFrameworkDetectorWithConfig(DefaultFrameworkDetectorConfig())
}

// NewFrameworkDetectorForModule creates a detector that classifies project
// packages by the given module path (as read from go.mod). Pass "" only when
// there is no module to read — the heuristic fallback is markedly less precise.
func NewFrameworkDetectorForModule(modulePath string) *FrameworkDetector {
	fd := NewFrameworkDetector()
	fd.modulePath = modulePath
	return fd
}

// NewFrameworkDetectorWithConfig creates a new framework detector with custom configuration
func NewFrameworkDetectorWithConfig(config FrameworkDetectorConfig) *FrameworkDetector {
	return &FrameworkDetector{
		config:                 config,
		packages:               make(map[string]*packages.Package, 100), // Pre-allocate with estimated capacity
		dependencyGraph:        make(map[string][]string, 50),
		reverseDependencyGraph: make(map[string][]string, 50),
	}
}

// frameworkPatternsFromRegistry projects the framework registry into the
// import-pattern map this detector classifies packages with.
func frameworkPatternsFromRegistry() map[string][]string {
	out := make(map[string][]string, len(core.Frameworks()))
	for _, fw := range core.Frameworks() {
		out[fw.DependencyKey] = slices.Clone(fw.ImportPatterns)
	}
	return out
}

// frameworkExternalPrefixes returns the framework half of ExternalPrefixes, in
// detection-rank order. The remaining prefixes are ordinary vendor namespaces
// with no framework meaning and stay listed at the call site.
func frameworkExternalPrefixes() []string {
	var out []string
	for _, fw := range core.FrameworksByDetectionRank() {
		out = append(out, fw.ExternalPrefixes...)
	}
	return out
}

// DefaultFrameworkDetectorConfig returns the default configuration for framework detection
func DefaultFrameworkDetectorConfig() FrameworkDetectorConfig {
	return FrameworkDetectorConfig{
		FrameworkPatterns: frameworkPatternsFromRegistry(),
		ExternalPrefixes: append(frameworkExternalPrefixes(), []string{
			"golang.org/x/",
			"google.golang.org/",
			"go.uber.org/",
			"github.com/sirupsen/logrus",
			"github.com/spf13/",
			"github.com/stretchr/",
			"gorm.io/",
			"gopkg.in/",
			"k8s.io/",
			"sigs.k8s.io/",
			"github.com/google/uuid",
		}...),
		ProjectPatterns: []string{
			"/modules/",
			"/pkg/",
			"/internal/",
			"/api/",
			"/handlers/",
			"/models/",
			"/services/",
			"/repositories/",
			"/usecase/",
			"/domain/",
			"/dtos/",
			"/middleware/",
			"/config/",
			"/utils/",
			"/common/",
			"/constants/",
			"/web/",
			"/dto/",
			"/auth/",
			"/user/",
			"/handler/",
		},
		TestMockPatterns: []string{
			"/mock/", "/mocks/", "/test/", "/tests/",
			"/fake/", "/fakes/", "/stub/", "/stubs/",
			"mock", "fake", "stub", "mocked",
			"_mock", "_mocks", "_test", "_tests",
			"_fake", "_fakes", "_stub", "_stubs",
		},
		IncludeExternalPackages: false,
		MaxImportDepth:          3,
		DisabledFrameworks:      make(map[string]bool),
	}
}

// Configure sets configuration options for the framework detector
func (fd *FrameworkDetector) Configure(includeExternal bool, maxDepth int) {
	fd.config.IncludeExternalPackages = includeExternal
	fd.config.MaxImportDepth = maxDepth
}

// DisableFramework disables detection for a given framework type key (e.g., "http")
func (fd *FrameworkDetector) DisableFramework(frameworkType string) {
	if fd.config.DisabledFrameworks == nil {
		fd.config.DisabledFrameworks = make(map[string]bool)
	}
	fd.config.DisabledFrameworks[frameworkType] = true
}

// AnalyzeFrameworkDependencies analyzes all framework dependencies
func (fd *FrameworkDetector) AnalyzeFrameworkDependencies(
	pkgs []*packages.Package,
	pkgsMetadata map[string]map[string]*ast.File,
	fileToInfo map[*ast.File]*types.Info,
	fset *token.FileSet,
) (*FrameworkDependencyList, error) {
	// Build package map
	for _, pkg := range pkgs {
		fd.packages[pkg.PkgPath] = pkg
	}

	// Build dependency graph from filtered syntax (file-level aware)
	fd.buildDependencyGraph(pkgs)

	// Find all framework-related packages (direct + deep dependencies)
	allFrameworkPackages := fd.findAllFrameworkPackages(pkgs, pkgsMetadata, fileToInfo)

	// Create flat list
	list := &FrameworkDependencyList{
		AllPackages:      allFrameworkPackages,
		FrameworkTypes:   make(map[string][]string),
		TotalPackages:    len(allFrameworkPackages),
		DirectPackages:   0,
		IndirectPackages: 0,
	}

	// Group by framework type and count direct/indirect
	for _, dep := range allFrameworkPackages {
		list.FrameworkTypes[dep.FrameworkType] = append(list.FrameworkTypes[dep.FrameworkType], dep.PackagePath)
		if dep.IsDirect {
			list.DirectPackages++
		} else {
			list.IndirectPackages++
		}
	}

	// Deliberately silent: the caller reports these counts (engine's
	// "framework dependencies analysed" phase line). Printed here, they read as
	// a result — "Found 1 framework packages" is stated even when detection
	// produced nothing usable, which is half of why an unmatched router looked
	// like a success (issue #379).
	return list, nil
}

// buildDependencyGraph builds the dependency graph from packages
func (fd *FrameworkDetector) buildDependencyGraph(pkgs []*packages.Package) {
	for _, pkg := range pkgs {
		pkgPath := pkg.PkgPath
		fd.dependencyGraph[pkgPath] = make([]string, 0)
		fd.reverseDependencyGraph[pkgPath] = make([]string, 0)

		// Add direct dependencies based on filtered file syntax imports
		for _, file := range pkg.Syntax {
			for _, imp := range file.Imports {
				if imp.Path != nil {
					depPath := strings.Trim(imp.Path.Value, "\"")
					if depPath == "" {
						continue
					}
					fd.dependencyGraph[pkgPath] = append(fd.dependencyGraph[pkgPath], depPath)
					fd.reverseDependencyGraph[depPath] = append(fd.reverseDependencyGraph[depPath], pkgPath)
				}
			}
		}
	}
}

// findAllFrameworkPackages finds all framework-related packages (direct + deep dependencies)
func (fd *FrameworkDetector) findAllFrameworkPackages(
	pkgs []*packages.Package,
	pkgsMetadata map[string]map[string]*ast.File,
	fileToInfo map[*ast.File]*types.Info,
) []*FrameworkDependency {

	allPackages := make([]*FrameworkDependency, 0)
	processed := make(map[string]bool)

	// First, find all direct framework packages
	directFrameworkPackages := make(map[string]*FrameworkDependency)

	for _, pkg := range pkgs {
		pkgPath := pkg.PkgPath

		// Skip mock/test packages
		if fd.isTestMockPackage(pkgPath) {
			continue
		}

		// Check if this package directly imports any framework
		frameworkType := fd.detectFrameworkType(pkg)
		if frameworkType != "" {
			dep := &FrameworkDependency{
				PackagePath:   pkgPath,
				FrameworkType: frameworkType,
				IsDirect:      true,
				Files:         make([]string, 0),
				Functions:     make([]string, 0),
				Types:         make([]string, 0),
				Metadata:      make(map[string]interface{}),
			}

			// Analyze package contents
			fd.analyzePackageContents(pkg, dep, pkgsMetadata, fileToInfo)

			directFrameworkPackages[pkgPath] = dep
			allPackages = append(allPackages, dep)
			processed[pkgPath] = true
		}
	}

	// Then, find all packages that depend on framework packages (deep dependencies)
	for _, pkg := range pkgs {
		pkgPath := pkg.PkgPath

		if processed[pkgPath] {
			continue // Already processed as direct framework package
		}

		// Skip mock/test packages
		if fd.isTestMockPackage(pkgPath) {
			continue
		}

		// Check if this package depends on any framework package
		if fd.dependsOnFrameworkPackage(pkgPath, directFrameworkPackages) {
			dep := &FrameworkDependency{
				PackagePath:   pkgPath,
				FrameworkType: "dependent",
				IsDirect:      false,
				Files:         make([]string, 0),
				Functions:     make([]string, 0),
				Types:         make([]string, 0),
				Metadata:      make(map[string]interface{}),
			}

			// Analyze package contents
			fd.analyzePackageContents(pkg, dep, pkgsMetadata, fileToInfo)

			allPackages = append(allPackages, dep)
			processed[pkgPath] = true
		}
	}

	// Finally, find all packages that are imported by framework packages (imported dependencies)
	importedPackages := fd.findImportedPackages(directFrameworkPackages, pkgs, processed)
	for _, dep := range importedPackages {
		allPackages = append(allPackages, dep)
		processed[dep.PackagePath] = true
	}

	return allPackages
}

// frameworkDetectionOrder returns the order detectFrameworkType checks
// FrameworkPatterns: known specific frameworks first, then any custom keys
// (sorted), with the generic "http" bucket always last. Ranging the map
// directly made the winner random per run for packages that import both a
// framework and net/http (nearly all handler code does).
func (fd *FrameworkDetector) frameworkDetectionOrder() []string {
	seen := map[string]bool{core.StdlibDependencyKey: true}
	order := make([]string, 0, len(fd.config.FrameworkPatterns))
	for _, fw := range core.FrameworksByDetectionRank() {
		k := fw.DependencyKey
		if seen[k] {
			continue // the stdlib bucket is appended last, below
		}
		if _, ok := fd.config.FrameworkPatterns[k]; ok {
			order = append(order, k)
			seen[k] = true
		}
	}
	var extras []string
	for k := range fd.config.FrameworkPatterns {
		if !seen[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	order = append(order, extras...)
	if _, ok := fd.config.FrameworkPatterns[core.StdlibDependencyKey]; ok {
		order = append(order, core.StdlibDependencyKey)
	}
	return order
}

// detectFrameworkType detects which framework this package uses
func (fd *FrameworkDetector) detectFrameworkType(pkg *packages.Package) string {
	for _, frameworkType := range fd.frameworkDetectionOrder() {
		patterns := fd.config.FrameworkPatterns[frameworkType]
		if fd.config.DisabledFrameworks[frameworkType] {
			continue
		}
		for _, pattern := range patterns {
			// Check imports at file level to respect filtered files
			for _, file := range pkg.Syntax {
				for _, imp := range file.Imports {
					if imp.Path != nil {
						importPath := strings.Trim(imp.Path.Value, "\"")
						if strings.HasPrefix(importPath, pattern) {
							return frameworkType
						}
					}
				}
			}
		}
	}
	return ""
}

// dependsOnFrameworkPackage checks if a package depends on any framework package
func (fd *FrameworkDetector) dependsOnFrameworkPackage(
	pkgPath string,
	frameworkPackages map[string]*FrameworkDependency,
) bool {
	// Check direct dependencies
	for _, depPath := range fd.dependencyGraph[pkgPath] {
		if _, isFramework := frameworkPackages[depPath]; isFramework {
			return true
		}
	}

	// Check transitive dependencies (deep search)
	visited := make(map[string]bool)
	return fd.hasTransitiveFrameworkDependency(pkgPath, frameworkPackages, visited)
}

// hasTransitiveFrameworkDependency checks for transitive framework dependencies
func (fd *FrameworkDetector) hasTransitiveFrameworkDependency(
	pkgPath string,
	frameworkPackages map[string]*FrameworkDependency,
	visited map[string]bool,
) bool {
	if visited[pkgPath] {
		return false // Avoid cycles
	}
	visited[pkgPath] = true

	// Check direct dependencies
	for _, depPath := range fd.dependencyGraph[pkgPath] {
		if _, isFramework := frameworkPackages[depPath]; isFramework {
			return true
		}

		// Recursively check transitive dependencies
		if fd.hasTransitiveFrameworkDependency(depPath, frameworkPackages, visited) {
			return true
		}
	}

	return false
}

// findImportedPackages finds all packages that are imported by framework packages (recursively)
func (fd *FrameworkDetector) findImportedPackages(
	directFrameworkPackages map[string]*FrameworkDependency,
	pkgs []*packages.Package,
	processed map[string]bool,
) []*FrameworkDependency {

	importedPackages := make([]*FrameworkDependency, 0)
	importedPackagePaths := make(map[string]bool)

	// Create a map of all available packages for quick lookup
	availablePackages := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		availablePackages[pkg.PkgPath] = pkg
	}

	// For each framework package, find all its imports recursively
	for _, frameworkDep := range directFrameworkPackages {
		pkgPath := frameworkDep.PackagePath

		if pkg, exists := availablePackages[pkgPath]; exists {
			// Recursively find all imports
			fd.findImportsRecursively(pkg, availablePackages, importedPackagePaths, processed, &importedPackages)
		}
	}

	return importedPackages
}

// findImportsRecursively recursively finds all imports of a package
func (fd *FrameworkDetector) findImportsRecursively(
	pkg *packages.Package,
	availablePackages map[string]*packages.Package,
	importedPackagePaths map[string]bool,
	processed map[string]bool,
	importedPackages *[]*FrameworkDependency,
) {
	fd.findImportsRecursivelyWithDepth(pkg, availablePackages, importedPackagePaths, processed, importedPackages, 0)
}

// findImportsRecursivelyWithDepth recursively finds all imports of a package with depth control
func (fd *FrameworkDetector) findImportsRecursivelyWithDepth(
	pkg *packages.Package,
	availablePackages map[string]*packages.Package,
	importedPackagePaths map[string]bool,
	processed map[string]bool,
	importedPackages *[]*FrameworkDependency,
	depth int,
) {
	// Check depth limit
	if depth >= fd.config.MaxImportDepth {
		return
	}
	// Extract imports from all files in this package
	for _, file := range pkg.Syntax {
		for _, imp := range file.Imports {
			if imp.Path != nil {
				importPath := strings.Trim(imp.Path.Value, "\"")

				// Skip if already processed
				if processed[importPath] || importedPackagePaths[importPath] {
					continue
				}

				// Skip standard library packages (packages without domain/namespace)
				// Standard library packages are typically single words like "fmt", "net", "os", etc.
				// Project packages typically have slashes like "complex-chi-router/models"
				if !strings.Contains(importPath, "/") && !strings.Contains(importPath, ".") {
					continue
				}

				// Check if this import should be included based on configuration
				shouldInclude := false
				if fd.config.IncludeExternalPackages {
					shouldInclude = true // Include all packages if external packages are allowed
				} else {
					shouldInclude = fd.isProjectRelatedPackage(importPath) // Only project-related packages
				}

				if shouldInclude {
					dep := &FrameworkDependency{
						PackagePath:   importPath,
						FrameworkType: "imported",
						IsDirect:      false,
						Files:         make([]string, 0),
						Functions:     make([]string, 0),
						Types:         make([]string, 0),
						Metadata:      make(map[string]interface{}),
					}

					// Check if this imported package exists in our available packages
					if importedPkg, exists := availablePackages[importPath]; exists {
						// Analyze package contents with full metadata
						fd.analyzePackageContents(importedPkg, dep, nil, nil)
					} else {
						// Package not in available packages, but still include it
						// This handles cases where project packages are imported but not in the original analysis
						dep.Metadata["note"] = "package not in original analysis"
						dep.Metadata["imported_by"] = pkg.PkgPath
					}

					*importedPackages = append(*importedPackages, dep)
					importedPackagePaths[importPath] = true

					// Recursively find imports of this package with increased depth
					// Only if the package exists in available packages
					if importedPkg, exists := availablePackages[importPath]; exists {
						fd.findImportsRecursivelyWithDepth(importedPkg, availablePackages, importedPackagePaths, processed, importedPackages, depth+1)
					}
				}
			}
		}
	}
}

// isTestMockPackage checks if a package is a test or mock package
func (fd *FrameworkDetector) isTestMockPackage(pkgPath string) bool {
	lowerPath := strings.ToLower(pkgPath)
	for _, pattern := range fd.config.TestMockPatterns {
		if strings.Contains(lowerPath, pattern) || strings.HasSuffix(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// isProjectRelatedPackage checks if a package is related to the current project
func (fd *FrameworkDetector) isProjectRelatedPackage(importPath string) bool {
	// Skip mock/test packages
	if fd.isTestMockPackage(importPath) {
		return false
	}

	// Skip external packages that are clearly not part of the project
	for _, prefix := range fd.config.ExternalPrefixes {
		if strings.HasPrefix(importPath, prefix) {
			return false
		}
	}

	// Use intelligent project package detection
	return fd.isIntelligentProjectPackage(importPath)
}

// isIntelligentProjectPackage determines whether a package belongs to the
// project. With a module path this is an exact answer; without one it degrades
// to inference over import paths.
func (fd *FrameworkDetector) isIntelligentProjectPackage(importPath string) bool {
	// go.mod already says which packages are ours. Guessing when the answer is
	// available was not merely redundant: for any domain-hosted module the
	// inference below returns no root at all (the common prefix contains a dot,
	// and no package path starts with a dot-free segment), so every third-party
	// import that is not in ExternalPrefixes fell through to
	// fallbackProjectPackageDetection and was classified as a project package
	// on the strength of having two slashes in it (issue #282).
	if fd.modulePath != "" {
		return importPath == fd.modulePath || strings.HasPrefix(importPath, fd.modulePath+"/")
	}

	// No module to read. Infer, and say so.
	projectRoot := fd.detectProjectRoot()
	if projectRoot == "" {
		return fd.fallbackProjectPackageDetection(importPath)
	}
	if importPath == projectRoot || strings.HasPrefix(importPath, projectRoot+"/") {
		return true
	}
	// Packages that are part of the project but not under the inferred root.
	return fd.isPackageImportedByProject(importPath)
}

// detectProjectRoot infers a project root as the longest common path prefix of
// the analysed packages. Only used when no module path is available; see
// isIntelligentProjectPackage.
func (fd *FrameworkDetector) detectProjectRoot() string {
	if len(fd.packages) == 0 {
		return ""
	}

	// Collect all package paths
	var packagePaths []string
	for pkgPath := range fd.packages {
		packagePaths = append(packagePaths, pkgPath)
	}

	if len(packagePaths) == 0 {
		return ""
	}

	// Find the longest common prefix among all package paths
	// This should give us the project root
	commonPrefix := packagePaths[0]

	for _, path := range packagePaths[1:] {
		commonPrefix = fd.findCommonPrefix(commonPrefix, path)
		if commonPrefix == "" {
			break
		}
	}

	// A bare host ("github.com") is not a project root — it would make every
	// package on that host project-related. Anything shorter than one segment
	// past the host is no root at all.
	segments := strings.Split(commonPrefix, "/")
	if commonPrefix == "" || (len(segments) == 1 && strings.Contains(segments[0], ".")) {
		return ""
	}

	return commonPrefix
}

// findCommonPrefix returns the longest common prefix of two import paths,
// measured in whole path segments. Comparing bytes instead produced strings
// that are not package paths at all — "github.com/acme/api" and
// "github.com/acme/app" share the bytes "github.com/acme/ap" — which were then
// used as a HasPrefix membership test (issue #282).
func (fd *FrameworkDetector) findCommonPrefix(a, b string) string {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	n := min(len(as), len(bs))
	i := 0
	for ; i < n && as[i] == bs[i]; i++ {
	}
	return strings.Join(as[:i], "/")
}

// isPackageImportedByProject checks if a package is imported by any of the analyzed project packages
func (fd *FrameworkDetector) isPackageImportedByProject(importPath string) bool {
	// Check if this package is imported by any of our analyzed packages
	for _, pkg := range fd.packages {
		// Check direct imports
		for _, file := range pkg.Syntax {
			for _, imp := range file.Imports {
				if imp.Path != nil {
					impPath := strings.Trim(imp.Path.Value, "\"")
					if impPath == importPath {
						return true
					}
				}
			}
		}
	}
	return false
}

// fallbackProjectPackageDetection provides a fallback when intelligent detection fails
func (fd *FrameworkDetector) fallbackProjectPackageDetection(importPath string) bool {
	// Include packages that look like they belong to the project
	// (contain common project patterns)
	for _, pattern := range fd.config.ProjectPatterns {
		if strings.Contains(importPath, pattern) {
			return true
		}
	}

	// Check if this looks like a project package by analyzing the structure
	// Project packages typically have patterns like: project-name/package-name
	parts := strings.Split(importPath, "/")
	if len(parts) >= 2 {
		// Check if it looks like a project package (not a standard library or external)
		// Examples: complex-chi-router/models, myproject/auth, etc.
		firstPart := parts[0]

		// If it contains hyphens or underscores, it's likely a project package
		if strings.Contains(firstPart, "-") || strings.Contains(firstPart, "_") {
			return true
		}

		// If it's a simple two-part package that doesn't look like a domain
		if len(parts) == 2 && !strings.Contains(firstPart, ".") {
			return true
		}
	}

	// If it doesn't match external prefixes and has a reasonable structure, include it
	// This is a fallback for project-specific packages
	return strings.Count(importPath, "/") >= 2 && !strings.Contains(importPath, "vendor/")
}

// analyzePackageContents analyzes the contents of a framework package
func (fd *FrameworkDetector) analyzePackageContents(
	pkg *packages.Package,
	dep *FrameworkDependency,
	pkgsMetadata map[string]map[string]*ast.File,
	fileToInfo map[*ast.File]*types.Info,
) {
	// Get files for this package, in sorted order — dep.Files and the
	// Functions/Types appended by analyzeFileContents are serialized with the
	// metadata, so map-range order would flip them per run.
	if files, ok := pkgsMetadata[pkg.PkgPath]; ok {
		for _, fileName := range slices.Sorted(maps.Keys(files)) {
			file := files[fileName]
			dep.Files = append(dep.Files, fileName)

			// Analyze file contents
			if _, ok := fileToInfo[file]; ok {
				fd.analyzeFileContents(file, dep)
			}
		}
	}

	// Add package metadata
	dep.Metadata["syntax_errors"] = len(pkg.Errors)
	dep.Metadata["imports_count"] = len(pkg.Imports)
	dep.Metadata["files_count"] = len(pkg.GoFiles)
}

// analyzeFileContents analyzes the contents of a file
func (fd *FrameworkDetector) analyzeFileContents(
	file *ast.File,
	dep *FrameworkDependency,
) {
	// Find functions
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return true
		}

		switch n := node.(type) {
		case *ast.FuncDecl:
			if n.Name != nil {
				funcName := n.Name.Name
				if !fd.contains(dep.Functions, funcName) {
					dep.Functions = append(dep.Functions, funcName)
				}
			}
		case *ast.TypeSpec:
			if n.Name != nil {
				typeName := n.Name.Name
				if !fd.contains(dep.Types, typeName) {
					dep.Types = append(dep.Types, typeName)
				}
			}
		}
		return true
	})
}

// PrintDependencyList prints the dependency list in a readable format
func (list *FrameworkDependencyList) PrintDependencyList() {
	fmt.Printf("\nFramework Dependency List\n")
	fmt.Printf("========================\n")
	fmt.Printf("Total Packages: %d\n", list.TotalPackages)
	fmt.Printf("Direct Packages: %d\n", list.DirectPackages)
	fmt.Printf("Indirect Packages: %d\n", list.IndirectPackages)

	// Group by framework type
	for frameworkType, packages := range list.FrameworkTypes {
		fmt.Printf("\n%s Framework (%d packages):\n", strings.ToUpper(frameworkType), len(packages))
		for _, pkgPath := range packages {
			// Find the dependency info
			var dep *FrameworkDependency
			for _, d := range list.AllPackages {
				if d.PackagePath == pkgPath {
					dep = d
					break
				}
			}

			if dep != nil {
				fmt.Printf("  %s (direct: %t, files: %d, functions: %d)\n",
					pkgPath, dep.IsDirect, len(dep.Files), len(dep.Functions))
			}
		}
	}

	// Show imported packages separately
	importedPackages := list.GetImportedPackages()
	if len(importedPackages) > 0 {
		fmt.Printf("\nIMPORTED Packages (%d packages):\n", len(importedPackages))
		for _, dep := range importedPackages {
			fmt.Printf("  %s (files: %d, functions: %d)\n",
				dep.PackagePath, len(dep.Files), len(dep.Functions))
		}
	}
}

// contains checks if a slice contains a string
func (fd *FrameworkDetector) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetFrameworkPackages returns packages grouped by framework type
func (list *FrameworkDependencyList) GetFrameworkPackages() map[string][]*FrameworkDependency {
	result := make(map[string][]*FrameworkDependency)

	for _, dep := range list.AllPackages {
		if dep.FrameworkType != "dependent" {
			result[dep.FrameworkType] = append(result[dep.FrameworkType], dep)
		}
	}

	// Sort packages within each framework type
	for frameworkType := range result {
		sort.Slice(result[frameworkType], func(i, j int) bool {
			return result[frameworkType][i].PackagePath < result[frameworkType][j].PackagePath
		})
	}

	return result
}

// GetImportedPackages returns only imported packages
func (list *FrameworkDependencyList) GetImportedPackages() []*FrameworkDependency {
	var importedPackages []*FrameworkDependency
	for _, dep := range list.AllPackages {
		if dep.FrameworkType == "imported" {
			importedPackages = append(importedPackages, dep)
		}
	}
	return importedPackages
}

// GetDirectPackages returns only direct framework packages
func (list *FrameworkDependencyList) GetDirectPackages() []*FrameworkDependency {
	var directPackages []*FrameworkDependency
	for _, dep := range list.AllPackages {
		if dep.IsDirect {
			directPackages = append(directPackages, dep)
		}
	}
	return directPackages
}

// GetIndirectPackages returns only indirect framework packages
func (list *FrameworkDependencyList) GetIndirectPackages() []*FrameworkDependency {
	var indirectPackages []*FrameworkDependency
	for _, dep := range list.AllPackages {
		if !dep.IsDirect {
			indirectPackages = append(indirectPackages, dep)
		}
	}
	return indirectPackages
}
