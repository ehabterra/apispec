package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

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
// This configuration allows for flexible and customizable framework detection
// without hardcoded values, making the system adaptable to different project structures.
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
// providing intelligent project package detection without hardcoded values.
type FrameworkDetector struct {
	config FrameworkDetectorConfig
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

// NewFrameworkDetectorWithConfig creates a new framework detector with custom configuration
func NewFrameworkDetectorWithConfig(config FrameworkDetectorConfig) *FrameworkDetector {
	return &FrameworkDetector{
		config:                 config,
		packages:               make(map[string]*packages.Package),
		dependencyGraph:        make(map[string][]string),
		reverseDependencyGraph: make(map[string][]string),
	}
}

// DefaultFrameworkDetectorConfig returns the default configuration for framework detection
func DefaultFrameworkDetectorConfig() FrameworkDetectorConfig {
	return FrameworkDetectorConfig{
		FrameworkPatterns: map[string][]string{
			"gin": {
				"github.com/gin-gonic/gin",
				"github.com/gin-contrib/",
			},
			"echo": {
				"github.com/labstack/echo",
				"github.com/labstack/echo/v4",
			},
			"fiber": {
				"github.com/gofiber/fiber",
				"github.com/gofiber/fiber/v2",
			},
			"chi": {
				"github.com/go-chi/chi",
				"github.com/go-chi/chi/v5",
			},
			"mux": {
				"github.com/gorilla/mux",
			},
			"http": {
				"net/http",
			},
			"fasthttp": {
				"github.com/valyala/fasthttp",
			},
		},
		ExternalPrefixes: []string{
			"github.com/gin-gonic/gin",
			"github.com/labstack/echo",
			"github.com/gofiber/fiber",
			"github.com/go-chi/chi",
			"github.com/gorilla/mux",
			"github.com/valyala/fasthttp",
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
		},
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

// AddFrameworkPattern adds a new framework pattern for detection
func (fd *FrameworkDetector) AddFrameworkPattern(frameworkType string, patterns []string) {
	if fd.config.FrameworkPatterns == nil {
		fd.config.FrameworkPatterns = make(map[string][]string)
	}
	fd.config.FrameworkPatterns[frameworkType] = patterns
}

// AddExternalPrefix adds a new external package prefix to exclude
func (fd *FrameworkDetector) AddExternalPrefix(prefix string) {
	fd.config.ExternalPrefixes = append(fd.config.ExternalPrefixes, prefix)
}

// AddProjectPattern adds a new project pattern for fallback detection
func (fd *FrameworkDetector) AddProjectPattern(pattern string) {
	fd.config.ProjectPatterns = append(fd.config.ProjectPatterns, pattern)
}

// AddTestMockPattern adds a new test/mock pattern to exclude
func (fd *FrameworkDetector) AddTestMockPattern(pattern string) {
	fd.config.TestMockPatterns = append(fd.config.TestMockPatterns, pattern)
}

// GetConfig returns a copy of the current configuration
func (fd *FrameworkDetector) GetConfig() FrameworkDetectorConfig {
	return fd.config
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

	fmt.Printf("Found %d framework packages (%d direct, %d indirect)\n",
		list.TotalPackages, list.DirectPackages, list.IndirectPackages)

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

// detectFrameworkType detects which framework this package uses
func (fd *FrameworkDetector) detectFrameworkType(pkg *packages.Package) string {
	for frameworkType, patterns := range fd.config.FrameworkPatterns {
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

	fmt.Printf("Found %d imported packages by framework packages (including transitive imports)\n", len(importedPackages))

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

// isIntelligentProjectPackage uses context-aware analysis to determine if a package belongs to the project
func (fd *FrameworkDetector) isIntelligentProjectPackage(importPath string) bool {
	// Get the project root from the analyzed packages
	projectRoot := fd.detectProjectRoot()
	if projectRoot == "" {
		// Fallback to simple heuristics if we can't detect project root
		return fd.fallbackProjectPackageDetection(importPath)
	}

	// Check if this package is under the detected project root
	if strings.HasPrefix(importPath, projectRoot) {
		return true
	}

	// Check if this package is imported by any of our analyzed packages
	// This catches packages that are part of the project but not under the main root
	return fd.isPackageImportedByProject(importPath)
}

// detectProjectRoot analyzes the package paths to determine the common project root
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

	// If the common prefix is too short or looks like a domain, try a different approach
	if len(commonPrefix) < 3 || strings.Contains(commonPrefix, ".") {
		// Look for packages that don't start with a domain (github.com, etc.)
		for _, path := range packagePaths {
			parts := strings.Split(path, "/")
			if len(parts) >= 2 && !strings.Contains(parts[0], ".") {
				// This looks like a project package (e.g., "myproject/models")
				return parts[0]
			}
		}
		return ""
	}

	return commonPrefix
}

// findCommonPrefix finds the longest common prefix between two strings
func (fd *FrameworkDetector) findCommonPrefix(a, b string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}

	return a[:minLen]
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
	// Get files for this package
	if files, ok := pkgsMetadata[pkg.PkgPath]; ok {
		for fileName, file := range files {
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
