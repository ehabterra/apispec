// Package engine provides the core OpenAPI generation engine used by both
// the CLI and the generator package.
package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

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

	// Load and type-check packages
	fset := token.NewFileSet()
	fileToInfo := make(map[*ast.File]*types.Info)

	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:   e.config.moduleRoot,
		Fset:  fset,
		Tests: false, // Explicitly exclude test files to speed up processing
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("packages contain errors")
	}

	// Group files by package for metadata
	pkgsMetadata := make(map[string]map[string]*ast.File)
	importPaths := make(map[string]string)

	for _, pkg := range pkgs {
		pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)
		for i, f := range pkg.Syntax {
			pkgsMetadata[pkg.PkgPath][pkg.GoFiles[i]] = f
			fileToInfo[f] = pkg.TypesInfo
			importPaths[pkg.GoFiles[i]] = pkg.PkgPath
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

	// Prepare generator config
	generatorConfig := intspec.GeneratorConfig{
		OpenAPIVersion: e.config.OpenAPIVersion,
		Title:          e.config.Title,
		APIVersion:     e.config.APIVersion,
	}

	// Construct the tracker tree
	limits := intspec.TrackerLimits{
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
