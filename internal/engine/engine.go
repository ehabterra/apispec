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

	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/metadata"
	intspec "github.com/ehabterra/swagen/internal/spec"
	"github.com/ehabterra/swagen/spec"
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
	DefaultMaxNodesPerTree    = 10000
	DefaultMaxChildrenPerNode = 150
	DefaultMaxArgsPerFunction = 30
	DefaultMaxNestedArgsDepth = 50
	DefaultMetadataFile       = "metadata.yaml"
	CopyrightNotice           = "swagen - Copyright 2025 Ehab Terra"
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
	SwagenConfig       *spec.SwagenConfig // Direct config object (takes precedence over ConfigFile)
	OutputConfig       string
	WriteMetadata      bool
	SplitMetadata      bool
	DiagramPath        string
	MaxNodesPerTree    int
	MaxChildrenPerNode int
	MaxArgsPerFunction int
	MaxNestedArgsDepth int

	// Include/exclude filters
	IncludeFiles     []string
	IncludePackages  []string
	IncludeFunctions []string
	IncludeTypes     []string
	ExcludeFiles     []string
	ExcludePackages  []string
	ExcludeFunctions []string
	ExcludeTypes     []string
	SkipCGOPackages  bool

	moduleRoot string
}

// DefaultEngineConfig returns a new EngineConfig with default values
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		InputDir:           DefaultInputDir,
		OutputFile:         DefaultOutputFile,
		Title:              DefaultTitle,
		APIVersion:         DefaultAPIVersion,
		Description:        "",
		TermsOfService:     "",
		ContactName:        DefaultContactName,
		ContactURL:         DefaultContactURL,
		ContactEmail:       DefaultContactEmail,
		LicenseName:        "",
		LicenseURL:         "",
		OpenAPIVersion:     DefaultOpenAPIVersion,
		ConfigFile:         "",
		OutputConfig:       "",
		WriteMetadata:      false,
		SplitMetadata:      false,
		DiagramPath:        "",
		MaxNodesPerTree:    DefaultMaxNodesPerTree,
		MaxChildrenPerNode: DefaultMaxChildrenPerNode,
		MaxArgsPerFunction: DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth: DefaultMaxNestedArgsDepth,
	}
}

// Engine represents the OpenAPI generation engine
type Engine struct {
	config *EngineConfig
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
func (e *Engine) GenerateOpenAPI() (*spec.OpenAPISpec, error) {
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

			// Check if file should be included/excluded
			if !e.shouldIncludeFile(fileName) {
				continue
			}

			pkgsMetadata[pkg.PkgPath][fileName] = f
			fileToInfo[f] = pkg.TypesInfo
			importPaths[fileName] = pkg.PkgPath
		}
	}

	// Generate metadata
	meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	// Detect framework and load configuration
	detector := core.NewFrameworkDetector()
	framework, err := detector.Detect(e.config.moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to detect framework: %w", err)
	}

	var swagenConfig *spec.SwagenConfig
	if e.config.SwagenConfig != nil {
		// Use the directly provided config
		swagenConfig = e.config.SwagenConfig
	} else if e.config.ConfigFile != "" {
		// Load config from file
		swagenConfig, err = spec.LoadSwagenConfig(e.config.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		// Auto-detect framework and use defaults
		switch framework {
		case "gin":
			swagenConfig = spec.DefaultGinConfig()
		case "chi":
			swagenConfig = spec.DefaultChiConfig()
		case "echo":
			swagenConfig = spec.DefaultEchoConfig()
		case "fiber":
			swagenConfig = spec.DefaultFiberConfig()
		case "mux":
			swagenConfig = spec.DefaultMuxConfig()
		default:
			swagenConfig = spec.DefaultHTTPConfig() // fallback
		}
	}

	// Set info from configuration (only if not already set in SwagenConfig)
	if swagenConfig.Info.Title == "" {
		swagenConfig.Info.Title = e.config.Title
	}
	if swagenConfig.Info.Description == "" {
		desc := e.config.Description
		if !strings.HasSuffix(desc, FullLicenseNotice) {
			desc += FullLicenseNotice
		}
		swagenConfig.Info.Description = desc
	}
	if swagenConfig.Info.Version == "" {
		swagenConfig.Info.Version = e.config.APIVersion
	}
	if swagenConfig.Info.TermsOfService == "" {
		swagenConfig.Info.TermsOfService = e.config.TermsOfService
	}
	if swagenConfig.Info.Contact == nil {
		swagenConfig.Info.Contact = &intspec.Contact{
			Name:  e.config.ContactName,
			URL:   e.config.ContactURL,
			Email: e.config.ContactEmail,
		}
	}
	if swagenConfig.Info.License == nil {
		swagenConfig.Info.License = &intspec.License{
			Name: e.config.LicenseName,
			URL:  e.config.LicenseURL,
		}
	}

	// Merge CLI include/exclude patterns with loaded configuration
	e.mergeIncludeExcludePatterns(swagenConfig)

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
	}
	tree := intspec.NewTrackerTree(meta, limits)

	// Generate diagram if requested
	if e.config.DiagramPath != "" {
		// Use absolute path for diagram file
		diagramPath := e.config.DiagramPath
		if !filepath.IsAbs(diagramPath) {
			diagramPath = filepath.Join(e.config.moduleRoot, diagramPath)
		}

		err := intspec.GenerateCytoscapeHTML(tree.GetRoots(), diagramPath)
		if err != nil {
			return nil, fmt.Errorf("failed to generate diagram: %w", err)
		}
	}

	// Generate OpenAPI spec
	openAPISpec, err := intspec.MapMetadataToOpenAPI(tree, swagenConfig, generatorConfig)
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

		cfgYaml, err := yaml.Marshal(swagenConfig)
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
func (e *Engine) mergeIncludeExcludePatterns(config *spec.SwagenConfig) {
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
			if matched, _ := filepath.Match(pattern, pkgPath); matched {
				return false
			}
			// Also check with wildcards for nested paths
			if strings.Contains(pkgPath, strings.Replace(pattern, "*/", "", 1)) {
				return false
			}
		}
	}

	// If no include/exclude patterns specified, include everything (except CGO problematic)
	if len(e.config.IncludeFiles) == 0 && len(e.config.ExcludeFiles) == 0 &&
		len(e.config.IncludePackages) == 0 && len(e.config.ExcludePackages) == 0 {
		return true
	}

	// Check exclude patterns first (exclude takes precedence)
	for _, pattern := range e.config.ExcludePackages {
		if matched, _ := filepath.Match(pattern, pkgPath); matched {
			return false
		}
		// Also check if the pattern matches the last part of the package path
		parts := strings.Split(pkgPath, "/")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if matched, _ := filepath.Match(pattern, lastPart); matched {
				return false
			}
		}
	}

	// Check include patterns
	if len(e.config.IncludePackages) > 0 {
		for _, pattern := range e.config.IncludePackages {
			if matched, _ := filepath.Match(pattern, pkgPath); matched {
				return true
			}
			// Also check if the pattern matches the last part of the package path
			parts := strings.Split(pkgPath, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				if matched, _ := filepath.Match(pattern, lastPart); matched {
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
	if len(e.config.IncludeFiles) == 0 && len(e.config.ExcludeFiles) == 0 {
		return true
	}

	// Check exclude patterns first (exclude takes precedence)
	for _, pattern := range e.config.ExcludeFiles {
		if matched, _ := filepath.Match(pattern, fileName); matched {
			return false
		}
	}

	// Check include patterns
	if len(e.config.IncludeFiles) > 0 {
		for _, pattern := range e.config.IncludeFiles {
			if matched, _ := filepath.Match(pattern, fileName); matched {
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

	// If no filtering is specified, return all packages
	if len(e.config.IncludeFiles) == 0 && len(e.config.ExcludeFiles) == 0 &&
		len(e.config.IncludePackages) == 0 && len(e.config.ExcludePackages) == 0 {
		return pkgs, nil
	}

	// Filter packages based on include/exclude patterns
	var filteredPkgs []*packages.Package
	for _, pkg := range pkgs {
		if e.shouldIncludePackage(pkg.PkgPath) {
			// Filter files within the package
			var filteredFiles []string
			var filteredSyntax []*ast.File

			for i, file := range pkg.GoFiles {
				fileName := filepath.Base(file)
				if e.shouldIncludeFile(fileName) {
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
