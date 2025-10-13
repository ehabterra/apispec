// Package engine provides the core OpenAPI generation engine used by both
// the CLI and the generator package.
package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"go/types"

	"github.com/ehabterra/apispec/internal/core"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/pkg/patterns"
	"github.com/ehabterra/apispec/spec"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

const (
	// Default values for OpenAPI generation
	DefaultOutputFile         = "openapi.json"
	DefaultInputDir           = "."
	DefaultTitle              = "Generated API"
	DefaultAPIVersion         = "1.0.0"
	DefaultContactName        = "Ehab"
	DefaultContactURL         = "https://ehabterra.github.io/"
	DefaultContactEmail       = "ehabterra@hotmail.com"
	DefaultOpenAPIVersion     = "3.1.1"
	DefaultMaxNodesPerTree    = 50000
	DefaultMaxChildrenPerNode = 500
	DefaultMaxArgsPerFunction = 100
	DefaultMaxNestedArgsDepth = 100
	DefaultMaxRecursionDepth  = 10
	DefaultMetadataFile       = "metadata.yaml"
	CopyrightNotice           = "apispec - Copyright 2025 Ehab Terra"
	LicenseNotice             = "Licensed under the Apache License 2.0. See LICENSE and NOTICE."
	FullLicenseNotice         = "\n\nCopyright 2025 Ehab Terra. Licensed under the Apache License 2.0. See LICENSE and NOTICE."
)

// EngineConfig holds configuration for the OpenAPI generation engine
type EngineConfig struct {
	InputDir           string
	OutputFile         string
	Title              string
	APIVersion         string
	Description        string
	TermsOfService     string
	ContactName        string
	ContactURL         string
	ContactEmail       string
	LicenseName        string
	LicenseURL         string
	OpenAPIVersion     string
	ConfigFile         string
	APISpecConfig      *spec.APISpecConfig // Direct config object (takes precedence over ConfigFile)
	OutputConfig       string
	WriteMetadata      bool
	SplitMetadata      bool
	DiagramPath        string
	PaginatedDiagram   bool
	DiagramPageSize    int
	MaxNodesPerTree    int
	MaxChildrenPerNode int
	MaxArgsPerFunction int
	MaxNestedArgsDepth int
	MaxRecursionDepth  int

	// Include/exclude filters
	IncludeFiles                 []string
	IncludePackages              []string
	IncludeFunctions             []string
	IncludeTypes                 []string
	ExcludeFiles                 []string
	ExcludePackages              []string
	ExcludeFunctions             []string
	ExcludeTypes                 []string
	SkipCGOPackages              bool
	AnalyzeFrameworkDependencies bool
	AutoIncludeFrameworkPackages bool
	// SkipHTTPFramework excludes net/http from framework dependency analysis
	SkipHTTPFramework bool
	// Auto-exclude common test files and folders (e.g., *_test.go, tests/)
	AutoExcludeTests bool
	// Auto-exclude common mock files and folders (e.g., *_mock.go, mocks/)
	AutoExcludeMocks bool

	moduleRoot string
}

// DefaultEngineConfig returns a new EngineConfig with default values
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		InputDir:                     DefaultInputDir,
		OutputFile:                   DefaultOutputFile,
		Title:                        DefaultTitle,
		APIVersion:                   DefaultAPIVersion,
		Description:                  "",
		TermsOfService:               "",
		ContactName:                  DefaultContactName,
		ContactURL:                   DefaultContactURL,
		ContactEmail:                 DefaultContactEmail,
		LicenseName:                  "",
		LicenseURL:                   "",
		OpenAPIVersion:               DefaultOpenAPIVersion,
		ConfigFile:                   "",
		OutputConfig:                 "",
		WriteMetadata:                false,
		SplitMetadata:                false,
		DiagramPath:                  "",
		PaginatedDiagram:             true,
		DiagramPageSize:              100,
		MaxNodesPerTree:              DefaultMaxNodesPerTree,
		MaxChildrenPerNode:           DefaultMaxChildrenPerNode,
		MaxArgsPerFunction:           DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth:           DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:            DefaultMaxRecursionDepth,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: true,
		SkipHTTPFramework:            false,
		AutoExcludeTests:             true,
		AutoExcludeMocks:             true,
	}
}

// Engine represents the OpenAPI generation engine
type Engine struct {
	config   *EngineConfig
	metadata *metadata.Metadata
}

// NewEngine creates a new Engine with the given configuration
func NewEngine(config *EngineConfig) *Engine {
	defaultConfig := DefaultEngineConfig()

	if config != nil {
		// Merge provided config with defaults
		if config.InputDir == "" {
			config.InputDir = defaultConfig.InputDir
		}
		if config.OutputFile == "" {
			config.OutputFile = defaultConfig.OutputFile
		}
		if config.Title == "" {
			config.Title = defaultConfig.Title
		}
		if config.APIVersion == "" {
			config.APIVersion = defaultConfig.APIVersion
		}
		if config.ContactName == "" {
			config.ContactName = defaultConfig.ContactName
		}
		if config.ContactURL == "" {
			config.ContactURL = defaultConfig.ContactURL
		}
		if config.ContactEmail == "" {
			config.ContactEmail = defaultConfig.ContactEmail
		}
		if config.OpenAPIVersion == "" {
			config.OpenAPIVersion = defaultConfig.OpenAPIVersion
		}
		if config.MaxNodesPerTree == 0 {
			config.MaxNodesPerTree = defaultConfig.MaxNodesPerTree
		}
		if config.MaxChildrenPerNode == 0 {
			config.MaxChildrenPerNode = defaultConfig.MaxChildrenPerNode
		}
		if config.MaxArgsPerFunction == 0 {
			config.MaxArgsPerFunction = defaultConfig.MaxArgsPerFunction
		}
		if config.MaxNestedArgsDepth == 0 {
			config.MaxNestedArgsDepth = defaultConfig.MaxNestedArgsDepth
		}
	} else {
		config = defaultConfig
	}

	return &Engine{config: config}
}

// GenerateOpenAPI generates an OpenAPI specification from the configured input directory
// GenerateMetadataOnly generates only metadata and call graph without OpenAPI spec
// This is useful for diagram servers and other tools that only need the call graph
func (e *Engine) GenerateMetadataOnly() (*metadata.Metadata, error) {
	// Validate input directory
	targetPath, err := filepath.Abs(e.config.InputDir)
	if err != nil {
		return nil, fmt.Errorf("could not resolve input directory: %w", err)
	}

	// Validate that the input directory exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("input directory does not exist: %s", targetPath)
	}

	// Find module root (but don't change working directory)
	e.config.moduleRoot, err = e.findModuleRoot(targetPath)
	if err != nil {
		return nil, fmt.Errorf("could not find Go module: %w", err)
	}

	// Create file set and file info mapping for metadata generation
	fset := token.NewFileSet()
	fileToInfo := make(map[*ast.File]*types.Info)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:  e.config.moduleRoot,
		Fset: fset,
	}

	// Filter packages and files based on include/exclude patterns
	filteredPkgs, err := e.loadFilteredPackages(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load filtered packages: %w", err)
	}

	// Filter out packages with errors and continue with valid packages
	var validPkgs []*packages.Package
	var errorCount int

	for _, pkg := range filteredPkgs {
		if len(pkg.Errors) > 0 {
			errorCount++
			// Log errors but continue processing other packages
			fmt.Printf("Warning: Skipping package %s due to errors:\n", pkg.PkgPath)
			for _, err := range pkg.Errors {
				fmt.Printf("  - %s\n", err.Msg)
			}
			continue
		}
		validPkgs = append(validPkgs, pkg)
	}

	// If all packages have errors, that's a problem
	if len(validPkgs) == 0 {
		return nil, fmt.Errorf("no valid packages found - all %d packages contain errors", errorCount)
	}

	if errorCount > 0 {
		fmt.Printf("Info: Continuing analysis with %d valid packages (%d packages skipped due to errors)\n",
			len(validPkgs), errorCount)
	}

	// Use valid packages instead of all filtered packages
	filteredPkgs = validPkgs

	// Group files by package for metadata
	pkgsMetadata := make(map[string]map[string]*ast.File)
	importPaths := make(map[string]string)

	for _, pkg := range filteredPkgs {
		// Check if package should be included/excluded
		if !e.shouldIncludePackage(pkg.PkgPath) {
			continue
		}

		pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)
		for i, f := range pkg.Syntax {
			fileName := pkg.GoFiles[i]

			// Use module-relative paths for file filtering
			relFile := fileName
			if e.config.moduleRoot != "" {
				if r, err := filepath.Rel(e.config.moduleRoot, fileName); err == nil {
					relFile = r
				}
			}

			// Check if file should be included/excluded
			if !e.shouldIncludeFile(relFile) {
				continue
			}

			pkgsMetadata[pkg.PkgPath][fileName] = f
			fileToInfo[f] = pkg.TypesInfo
			importPaths[fileName] = pkg.PkgPath
		}
	}

	// Analyze framework dependencies BEFORE metadata generation
	if e.config.AnalyzeFrameworkDependencies {
		fmt.Println("Analyzing framework dependencies...")
		dependencyTree, err := e.analyzeFrameworkDependencies(validPkgs, pkgsMetadata, fileToInfo, fset)
		if err != nil {
			fmt.Printf("Warning: Failed to analyze framework dependencies: %v\n", err)
		} else {
			fmt.Printf("Framework dependency analysis completed: %d packages found\n", dependencyTree.TotalPackages)

			// Auto-include framework packages in IncludePackages if requested
			if e.config.AutoIncludeFrameworkPackages {
				e.autoIncludeFrameworkPackages(dependencyTree)

				// Re-filter packages to only include framework packages
				fmt.Println("Re-filtering packages to include only framework packages...")
				pkgsMetadata, fileToInfo, importPaths = e.filterToFrameworkPackages(
					pkgsMetadata, fileToInfo, importPaths, dependencyTree)
				fmt.Printf("Filtered to %d framework packages for metadata generation\n", len(pkgsMetadata))
			}
		}
	}

	// Generate metadata (now only on framework packages if auto-include is enabled)
	meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	// Store metadata in engine
	e.metadata = meta

	// Store framework dependency list in metadata
	if e.config.AnalyzeFrameworkDependencies {
		// Re-analyze if we filtered packages, or use existing analysis
		if e.config.AutoIncludeFrameworkPackages {
			// Re-analyze with filtered packages for accurate results
			dependencyTree, err := e.analyzeFrameworkDependencies(validPkgs, pkgsMetadata, fileToInfo, fset)
			if err == nil {
				meta.FrameworkDependencyList = dependencyTree
			}
		} else {
			// Use the original analysis
			dependencyTree, err := e.analyzeFrameworkDependencies(validPkgs, pkgsMetadata, fileToInfo, fset)
			if err == nil {
				meta.FrameworkDependencyList = dependencyTree
			}
		}
	}

	return meta, nil
}

func (e *Engine) GenerateOpenAPI() (*spec.OpenAPISpec, error) {
	// Generate metadata using the shared method
	meta, err := e.GenerateMetadataOnly()
	if err != nil {
		return nil, err
	}

	// Generate diagram if requested
	if e.config.DiagramPath != "" {
		// Use absolute path for diagram file
		diagramPath := e.config.DiagramPath
		if !filepath.IsAbs(diagramPath) {
			diagramPath = filepath.Join(e.config.moduleRoot, diagramPath)
		}

		// Choose between paginated and regular diagram based on configuration
		if e.config.PaginatedDiagram {
			// Use paginated visualization for better performance with large call graphs
			// This solves the 3997-edge performance problem by loading data progressively
			err := intspec.GeneratePaginatedCytoscapeHTML(meta, diagramPath, e.config.DiagramPageSize)
			if err != nil {
				return nil, fmt.Errorf("failed to generate paginated diagram: %w", err)
			}
		} else {
			// Use regular call graph visualization for smaller graphs
			err := intspec.GenerateCallGraphCytoscapeHTML(meta, diagramPath)
			if err != nil {
				return nil, fmt.Errorf("failed to generate diagram: %w", err)
			}
		}
	}

	// Framework dependency analysis is now handled in GenerateMetadataOnly()

	// Detect framework and load configuration
	detector := core.NewFrameworkDetector()
	framework, err := detector.Detect(e.config.moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to detect framework: %w", err)
	}

	var apispecConfig *spec.APISpecConfig
	if e.config.APISpecConfig != nil {
		// Use the directly provided config
		apispecConfig = e.config.APISpecConfig
	} else if e.config.ConfigFile != "" {
		// Load config from file
		apispecConfig, err = spec.LoadAPISpecConfig(e.config.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		// Auto-detect framework and use defaults
		switch framework {
		case "gin":
			apispecConfig = spec.DefaultGinConfig()
		case "chi":
			apispecConfig = spec.DefaultChiConfig()
		case "echo":
			apispecConfig = spec.DefaultEchoConfig()
		case "fiber":
			apispecConfig = spec.DefaultFiberConfig()
		case "mux":
			apispecConfig = spec.DefaultMuxConfig()
		default:
			apispecConfig = spec.DefaultHTTPConfig() // fallback
		}
	}

	// Set info from configuration (only if not already set in APISpecConfig)
	if apispecConfig.Info.Title == "" {
		apispecConfig.Info.Title = e.config.Title
	}
	if apispecConfig.Info.Description == "" {
		desc := e.config.Description
		if !strings.HasSuffix(desc, FullLicenseNotice) {
			desc += FullLicenseNotice
		}
		apispecConfig.Info.Description = desc
	}
	if apispecConfig.Info.Version == "" {
		apispecConfig.Info.Version = e.config.APIVersion
	}
	if apispecConfig.Info.TermsOfService == "" {
		apispecConfig.Info.TermsOfService = e.config.TermsOfService
	}
	if apispecConfig.Info.Contact == nil {
		apispecConfig.Info.Contact = &intspec.Contact{
			Name:  e.config.ContactName,
			URL:   e.config.ContactURL,
			Email: e.config.ContactEmail,
		}
	}
	if apispecConfig.Info.License == nil {
		apispecConfig.Info.License = &intspec.License{
			Name: e.config.LicenseName,
			URL:  e.config.LicenseURL,
		}
	}

	// Merge CLI include/exclude patterns with loaded configuration
	e.mergeIncludeExcludePatterns(apispecConfig)

	// Prepare generator config
	generatorConfig := intspec.GeneratorConfig{
		OpenAPIVersion: e.config.OpenAPIVersion,
		Title:          e.config.Title,
		APIVersion:     e.config.APIVersion,
	}

	// Construct the tracker tree
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    e.config.MaxNodesPerTree,
		MaxChildrenPerNode: e.config.MaxChildrenPerNode,
		MaxArgsPerFunction: e.config.MaxArgsPerFunction,
		MaxNestedArgsDepth: e.config.MaxNestedArgsDepth,
		MaxRecursionDepth:  e.config.MaxRecursionDepth,
	}
	tree := intspec.NewTrackerTree(meta, limits)

	// Generate OpenAPI spec
	openAPISpec, err := intspec.MapMetadataToOpenAPI(tree, apispecConfig, generatorConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAPI spec: %w", err)
	}

	// Handle metadata writing if requested
	if e.config.WriteMetadata {
		// Use absolute path for metadata file
		metadataPath := DefaultMetadataFile
		if !filepath.IsAbs(metadataPath) {
			metadataPath = filepath.Join(e.config.moduleRoot, metadataPath)
		}

		if e.config.SplitMetadata {
			if err := metadata.WriteSplitMetadata(meta, metadataPath); err != nil {
				return nil, fmt.Errorf("failed to write split metadata: %w", err)
			}
		} else {
			if err := metadata.WriteMetadata(meta, metadataPath); err != nil {
				return nil, fmt.Errorf("failed to write metadata: %w", err)
			}
		}
	}

	// Output effective config if requested
	if e.config.OutputConfig != "" {
		// Use absolute path for config output file
		configPath := e.config.OutputConfig
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(e.config.moduleRoot, configPath)
		}

		cfgYaml, err := yaml.Marshal(apispecConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal effective config: %w", err)
		}
		err = os.WriteFile(configPath, cfgYaml, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write effective config: %w", err)
		}
	}

	return openAPISpec, nil
}

// mergeIncludeExcludePatterns merges CLI include/exclude patterns with the loaded configuration
func (e *Engine) mergeIncludeExcludePatterns(config *spec.APISpecConfig) {
	// Merge include patterns
	if len(e.config.IncludeFiles) > 0 {
		config.Include.Files = append(config.Include.Files, e.config.IncludeFiles...)
	}
	if len(e.config.IncludePackages) > 0 {
		config.Include.Packages = append(config.Include.Packages, e.config.IncludePackages...)
	}
	if len(e.config.IncludeFunctions) > 0 {
		config.Include.Functions = append(config.Include.Functions, e.config.IncludeFunctions...)
	}
	if len(e.config.IncludeTypes) > 0 {
		config.Include.Types = append(config.Include.Types, e.config.IncludeTypes...)
	}

	// Merge exclude patterns
	if len(e.config.ExcludeFiles) > 0 {
		config.Exclude.Files = append(config.Exclude.Files, e.config.ExcludeFiles...)
	}
	if len(e.config.ExcludePackages) > 0 {
		config.Exclude.Packages = append(config.Exclude.Packages, e.config.ExcludePackages...)
	}
	if len(e.config.ExcludeFunctions) > 0 {
		config.Exclude.Functions = append(config.Exclude.Functions, e.config.ExcludeFunctions...)
	}
	if len(e.config.ExcludeTypes) > 0 {
		config.Exclude.Types = append(config.Exclude.Types, e.config.ExcludeTypes...)
	}
}

func (e *Engine) ModuleRoot() string {
	return e.config.moduleRoot
}

// findModuleRoot finds the root directory of a Go module by looking for go.mod
func (e *Engine) findModuleRoot(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	current := absPath
	for {
		goModPath := filepath.Join(current, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break // reached root
		}
		current = parent
	}

	return "", fmt.Errorf("no go.mod found in %s or any parent directory", startPath)
}

// matchesPattern checks if a path matches a gitignore-style pattern
func matchesPattern(pattern, path string) bool {
	return patterns.Match(pattern, path)
}

// shouldIncludePackage checks if a package should be included based on include/exclude patterns
func (e *Engine) shouldIncludePackage(pkgPath string) bool {
	// Auto-exclude known problematic CGO dependencies if enabled
	if e.config.SkipCGOPackages {
		cgoProblematicPatterns := []string{
			"*/tensorflow/*",     // TensorFlow C bindings
			"*/govips/*",         // VIPS image processing
			"*/opencv/*",         // OpenCV bindings
			"*/ffmpeg/*",         // FFmpeg bindings
			"*/sqlite3",          // SQLite3 CGO driver
			"*/go-sqlite3",       // Alternative SQLite3 driver
			"*/graft/tensorflow", // Specific TensorFlow graft package
		}

		for _, pattern := range cgoProblematicPatterns {
			if matchesPattern(pattern, pkgPath) {
				return false
			}
			// Also check with wildcards for nested paths
			if strings.Contains(pkgPath, strings.Replace(pattern, "*/", "", 1)) {
				return false
			}
		}
	}

	// Auto-exclude test/mock packages if enabled (case-insensitive)
	lowerPkg := strings.ToLower(pkgPath)
	if e.config.AutoExcludeTests {
		if strings.HasSuffix(lowerPkg, "_test") || strings.HasSuffix(lowerPkg, "_tests") {
			return false
		}
	}
	if e.config.AutoExcludeMocks {
		if strings.HasSuffix(lowerPkg, "mock") || strings.HasSuffix(lowerPkg, "mocks") ||
			strings.HasSuffix(lowerPkg, "fake") || strings.HasSuffix(lowerPkg, "fakes") ||
			strings.HasSuffix(lowerPkg, "stub") || strings.HasSuffix(lowerPkg, "stubs") {
			return false
		}
	}

	// If no include/exclude patterns specified, include everything (except CGO problematic)
	if len(e.config.IncludeFiles) == 0 && len(e.config.ExcludeFiles) == 0 &&
		len(e.config.IncludePackages) == 0 && len(e.config.ExcludePackages) == 0 {
		return true
	}

	// Check exclude patterns first (exclude takes precedence)
	for _, pattern := range e.config.ExcludePackages {
		if matchesPattern(pattern, pkgPath) {
			return false
		}
		// Also check if the pattern matches the last part of the package path
		parts := strings.Split(pkgPath, "/")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if matchesPattern(pattern, lastPart) {
				return false
			}
		}
	}

	// Check include patterns
	if len(e.config.IncludePackages) > 0 {
		for _, pattern := range e.config.IncludePackages {
			if matchesPattern(pattern, pkgPath) {
				return true
			}
			// Also check if the pattern matches the last part of the package path
			parts := strings.Split(pkgPath, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				if matchesPattern(pattern, lastPart) {
					return true
				}
			}
		}
		return false // Not matched by any include pattern
	}

	return true // No include patterns specified, so include
}

// shouldIncludeFile checks if a file should be included based on include/exclude patterns
func (e *Engine) shouldIncludeFile(fileName string) bool {
	// If no include/exclude patterns specified, include everything
	// But first apply auto excludes when enabled
	lower := strings.ToLower(fileName)
	if e.config.AutoExcludeTests {
		// Common test patterns
		if strings.HasSuffix(lower, "test.go") || strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") {
			return false
		}
	}
	if e.config.AutoExcludeMocks {
		// Common mock/fake/stub patterns - more comprehensive
		if strings.HasSuffix(lower, "mock.go") || strings.HasSuffix(lower, "fake.go") || strings.HasSuffix(lower, "stub.go") ||
			strings.HasSuffix(lower, "mocks.go") || strings.HasSuffix(lower, "fakes.go") || strings.HasSuffix(lower, "stubs.go") {
			return false
		}
	}

	// If no explicit patterns specified, return true (auto-excludes already applied above)
	if len(e.config.IncludeFiles) == 0 && len(e.config.ExcludeFiles) == 0 {
		return true
	}

	// Check exclude patterns first (exclude takes precedence)
	for _, pattern := range e.config.ExcludeFiles {
		if matchesPattern(pattern, fileName) {
			return false
		}
	}

	// Check include patterns
	if len(e.config.IncludeFiles) > 0 {
		for _, pattern := range e.config.IncludeFiles {
			if matchesPattern(pattern, fileName) {
				return true
			}
		}
		return false // Not matched by any include pattern
	}

	return true // No include patterns specified, so include
}

// loadFilteredPackages loads packages with filtering based on include/exclude patterns
func (e *Engine) loadFilteredPackages(cfg *packages.Config) ([]*packages.Package, error) {
	// Load all packages first to ensure proper Go module resolution
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}

	// Always apply auto-exclude logic, even if no explicit patterns are specified
	// This ensures mock/test files are excluded by default

	// Filter packages based on include/exclude patterns
	var filteredPkgs []*packages.Package
	for _, pkg := range pkgs {
		if e.shouldIncludePackage(pkg.PkgPath) {
			// Filter files within the package
			var filteredFiles []string
			var filteredSyntax []*ast.File

			for i, file := range pkg.GoFiles {
				// Use module-relative paths for file filtering to enable directory-aware patterns
				relFile := file
				if e.config.moduleRoot != "" {
					if r, err := filepath.Rel(e.config.moduleRoot, file); err == nil {
						relFile = r
					}
				}
				if e.shouldIncludeFile(relFile) {
					filteredFiles = append(filteredFiles, file)
					if i < len(pkg.Syntax) {
						filteredSyntax = append(filteredSyntax, pkg.Syntax[i])
					}
				}
			}

			// Only include package if it has files after filtering
			if len(filteredFiles) > 0 {
				// Create a copy of the package with filtered files
				filteredPkg := *pkg
				filteredPkg.GoFiles = filteredFiles
				filteredPkg.Syntax = filteredSyntax
				filteredPkgs = append(filteredPkgs, &filteredPkg)
			}
		}
	}

	return filteredPkgs, nil
}

// GetMetadata returns the current metadata
func (e *Engine) GetMetadata() *metadata.Metadata {
	return e.metadata
}

// GetConfig returns the current engine configuration
func (e *Engine) GetConfig() *EngineConfig {
	return e.config
}

// analyzeFrameworkDependencies analyzes framework dependencies
func (e *Engine) analyzeFrameworkDependencies(
	validPkgs []*packages.Package,
	pkgsMetadata map[string]map[string]*ast.File,
	fileToInfo map[*ast.File]*types.Info,
	fset *token.FileSet,
) (*metadata.FrameworkDependencyList, error) {
	detector := metadata.NewFrameworkDetector()
	// Configure detector for more precise analysis
	detector.Configure(false, 2) // Don't include external packages, max 2 levels deep
	if e.config.SkipHTTPFramework {
		detector.DisableFramework("http")
	}
	return detector.AnalyzeFrameworkDependencies(validPkgs, pkgsMetadata, fileToInfo, fset)
}

// autoIncludeFrameworkPackages automatically adds framework packages to IncludePackages
func (e *Engine) autoIncludeFrameworkPackages(frameworkList *metadata.FrameworkDependencyList) {
	if frameworkList == nil || len(frameworkList.AllPackages) == 0 {
		return
	}

	fmt.Println("Auto-including framework packages in IncludePackages...")

	// Create a map of existing include packages for quick lookup
	existingIncludes := make(map[string]bool)
	for _, pkg := range e.config.IncludePackages {
		existingIncludes[pkg] = true
	}

	// Add framework packages to IncludePackages
	addedCount := 0
	for _, dep := range frameworkList.AllPackages {
		if !existingIncludes[dep.PackagePath] {
			e.config.IncludePackages = append(e.config.IncludePackages, dep.PackagePath)
			existingIncludes[dep.PackagePath] = true
			addedCount++
		}
	}

	fmt.Printf("Added %d framework packages to IncludePackages\n", addedCount)
	fmt.Printf("Total IncludePackages: %d\n", len(e.config.IncludePackages))

	// Print the added packages
	if addedCount > 0 {
		fmt.Println("Added framework packages:")
		for _, dep := range frameworkList.AllPackages {
			if existingIncludes[dep.PackagePath] {
				frameworkType := dep.FrameworkType
				if dep.IsDirect {
					frameworkType += " (direct)"
				} else {
					frameworkType += " (indirect)"
				}
				fmt.Printf("  - %s (%s)\n", dep.PackagePath, frameworkType)
			}
		}
	}
}

// filterToFrameworkPackages filters packages to only include framework-related packages
func (e *Engine) filterToFrameworkPackages(
	pkgsMetadata map[string]map[string]*ast.File,
	fileToInfo map[*ast.File]*types.Info,
	importPaths map[string]string,
	frameworkList *metadata.FrameworkDependencyList,
) (map[string]map[string]*ast.File, map[*ast.File]*types.Info, map[string]string) {

	// Create a set of framework package paths for quick lookup
	frameworkPackages := make(map[string]bool)
	for _, dep := range frameworkList.AllPackages {
		frameworkPackages[dep.PackagePath] = true
	}

	// Filter packages metadata
	filteredPkgsMetadata := make(map[string]map[string]*ast.File)
	for pkgPath, files := range pkgsMetadata {
		if frameworkPackages[pkgPath] {
			filteredPkgsMetadata[pkgPath] = files
		}
	}

	// Filter file to info mapping
	filteredFileToInfo := make(map[*ast.File]*types.Info)
	for file, info := range fileToInfo {
		// Check if this file belongs to a framework package
		fileBelongsToFramework := false
		for _, files := range filteredPkgsMetadata {
			for _, pkgFile := range files {
				if pkgFile == file {
					fileBelongsToFramework = true
					break
				}
			}
			if fileBelongsToFramework {
				break
			}
		}

		if fileBelongsToFramework {
			filteredFileToInfo[file] = info
		}
	}

	// Filter import paths
	filteredImportPaths := make(map[string]string)
	for fileName, pkgPath := range importPaths {
		if frameworkPackages[pkgPath] {
			filteredImportPaths[fileName] = pkgPath
		}
	}

	return filteredPkgsMetadata, filteredFileToInfo, filteredImportPaths
}
