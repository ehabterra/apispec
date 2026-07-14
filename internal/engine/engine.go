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

// Package engine provides the core OpenAPI generation engine used by both
// the CLI and the generator package.
package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go/types"

	"github.com/ehabterra/apispec/internal/callgraph"
	"github.com/ehabterra/apispec/internal/core"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/pkg/patterns"
	"github.com/ehabterra/apispec/spec"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// VerboseLogger provides conditional logging based on verbose setting
type VerboseLogger struct {
	verbose bool
}

// NewVerboseLogger creates a new verbose logger
func NewVerboseLogger(verbose bool) *VerboseLogger {
	return &VerboseLogger{verbose: verbose}
}

// Printf prints formatted output only if verbose is enabled
func (vl *VerboseLogger) Printf(format string, args ...interface{}) {
	if vl.verbose {
		fmt.Printf(format, args...)
	}
}

// Println prints output only if verbose is enabled
func (vl *VerboseLogger) Println(args ...interface{}) {
	if vl.verbose {
		fmt.Println(args...)
	}
}

// Print prints output only if verbose is enabled
func (vl *VerboseLogger) Print(args ...interface{}) {
	if vl.verbose {
		fmt.Print(args...)
	}
}

// Warnf writes an always-on warning to stderr. Unlike Printf/Println/Print,
// it is not gated on the verbose flag: warnings about limit overruns or
// recoverable failures are surfaced to the consumer either way.
func (vl *VerboseLogger) Warnf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format, args...)
	if err != nil {
		return
	}
}

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
	CopyrightNotice           = "apispec - Copyright 2026 Ehab Terra"
	LicenseNotice             = "Licensed under the Apache License 2.0. See LICENSE and NOTICE."
	FullLicenseNotice         = "\n\nCopyright 2026 Ehab Terra. Licensed under the Apache License 2.0. See LICENSE and NOTICE."
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
	// ResolveCallGraph builds the SSA+VTA resolved call graph alongside
	// metadata (docs/TRACKER_REDESIGN.md step 2). Off by default until the
	// summary-based analyses consume it; enable to expose it via
	// GetResolvedCallGraph.
	ResolveCallGraph bool
	// UseLazyTracker generates the spec with the lazy tracker tree
	// (docs/TRACKER_REDESIGN.md step 4) — the default. It covers the wiring
	// styles the legacy eager tree supports (verified by the fixture parity
	// harness and the cross-codebase meter), resolves some responses/bodies
	// the eager tree misses, and stays bounded on dense call graphs. Set to
	// false (CLI --legacy-tracker, UI "Analysis engine") to generate with
	// the legacy eager tree for comparison.
	UseLazyTracker bool
	// SkipHTTPFramework excludes net/http from framework dependency analysis
	SkipHTTPFramework bool
	// Auto-exclude common test files and folders (e.g., *_test.go, tests/)
	AutoExcludeTests bool
	// Auto-exclude common mock files and folders (e.g., *_mock.go, mocks/)
	AutoExcludeMocks bool

	// Verbose output control
	Verbose bool

	// OnPhase, if set, is invoked at each major engine phase boundary with a
	// short stable identifier ("packages", "framework-deps", "metadata",
	// "spec") and the elapsed time for that phase. Always-on regardless of
	// Verbose — intended for UIs that want to surface live progress without
	// firehosing every debug log to the user.
	OnPhase func(phase string, elapsed time.Duration)

	// Context, if set, cancels generation. The slow package-load phase is
	// passed this context, and the engine aborts at each phase boundary
	// when it's cancelled — so a UI can stop a run in flight.
	Context context.Context

	moduleRoot string
}

// ctx returns the configured context or a background context.
func (e *Engine) ctx() context.Context {
	if e.config != nil && e.config.Context != nil {
		return e.config.Context
	}
	return context.Background()
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
		UseLazyTracker:               true,
	}
}

// reportPhase logs an engine phase boundary to stderr and invokes any
// OnPhase callback on the config. It's always-on so users running the UI or
// CLI can see *which* stage of analysis is taking time without flipping the
// verbose flag.
func (e *Engine) reportPhase(phase string, elapsed time.Duration) {
	if e == nil {
		return
	}
	log.Printf("[engine] %s in %s", phase, elapsed.Round(time.Millisecond))
	if e.config != nil && e.config.OnPhase != nil {
		// Defensive: don't let a misbehaving callback panic the analysis.
		defer func() { _ = recover() }()
		e.config.OnPhase(phase, elapsed)
	}
}

// Engine represents the OpenAPI generation engine
type Engine struct {
	config   *EngineConfig
	metadata *metadata.Metadata

	// skipped records packages dropped during analysis because they failed to
	// type-check (e.g. an unresolved/private dependency). Surfaced so callers
	// can warn that the spec may be incomplete. Keyed by package path → first
	// error message.
	skipped []SkippedPackage

	// unresolvedSecurity lists auth middleware detected during the last
	// generation that matched no SecurityMapping. Surfaced to callers (the UI)
	// so the user can map it to a scheme.
	unresolvedSecurity []intspec.MiddlewareRef

	// pathParamMismatches lists map-key path-variable reads (mux.Vars(r)["x"])
	// whose key matches no route placeholder, gathered during the last generation.
	pathParamMismatches []intspec.PathParamMismatch

	// resolvedGraph is the SSA+VTA resolved call graph, built during
	// GenerateMetadataOnly when config.ResolveCallGraph is set.
	resolvedGraph *callgraph.Resolved
}

// GetResolvedCallGraph returns the resolved call graph from the last
// generation, or nil when config.ResolveCallGraph was off.
func (e *Engine) GetResolvedCallGraph() *callgraph.Resolved {
	return e.resolvedGraph
}

// SkippedPackage is a package excluded from analysis due to compile/type
// errors, with a representative reason.
type SkippedPackage struct {
	Package string `json:"package"`
	Reason  string `json:"reason"`
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
	return e.GenerateMetadataOnlyWithLogger(NewVerboseLogger(e.config.Verbose))
}

// GenerateMetadataOnlyWithLogger generates only metadata and call graph without OpenAPI spec with a custom logger
func (e *Engine) GenerateMetadataOnlyWithLogger(logger *VerboseLogger) (*metadata.Metadata, error) {
	// Fold any include/exclude patterns carried on the APISpecConfig (e.g. set
	// via the UI or a config file) into the EngineConfig filter fields, which
	// shouldIncludePackage / shouldIncludeFile actually read. Without this the
	// config's Include/Exclude were silently ignored during analysis (only
	// CLI-flag patterns took effect).
	e.applyConfigFilters()

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
		// NeedCompiledGoFiles and NeedTypesSizes are required by the SSA
		// builder (config.ResolveCallGraph); harmless additions otherwise.
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:     e.config.moduleRoot,
		Fset:    fset,
		Context: e.ctx(),
	}

	// Filter packages and files based on include/exclude patterns
	t0 := time.Now()
	filteredPkgs, err := e.loadFilteredPackages(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load filtered packages: %w", err)
	}
	if err := e.ctx().Err(); err != nil {
		return nil, err
	}
	e.reportPhase(fmt.Sprintf("loaded %d packages", len(filteredPkgs)), time.Since(t0))

	// Filter out packages with errors and continue with valid packages
	var validPkgs []*packages.Package
	var errorCount int

	e.skipped = nil
	for _, pkg := range filteredPkgs {
		if len(pkg.Errors) > 0 {
			errorCount++
			// Log errors but continue processing other packages
			logger.Printf("Warning: Skipping package %s due to errors:\n", pkg.PkgPath)
			for _, err := range pkg.Errors {
				logger.Printf("  - %s\n", err.Msg)
			}
			// Record (only in-module packages — third-party type errors are
			// rarely actionable by the user) so the caller can surface them.
			if mp := e.moduleImportPath(); mp == "" || pkg.PkgPath == mp || strings.HasPrefix(pkg.PkgPath, mp+"/") {
				reason := ""
				if len(pkg.Errors) > 0 {
					reason = pkg.Errors[0].Msg
				}
				e.skipped = append(e.skipped, SkippedPackage{Package: pkg.PkgPath, Reason: reason})
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
		logger.Printf("Info: Continuing analysis with %d valid packages (%d packages skipped due to errors)\n",
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
	var dependencyTree *metadata.FrameworkDependencyList
	if e.config.AnalyzeFrameworkDependencies {
		logger.Println("Analyzing framework dependencies...")
		tDeps := time.Now()
		var err error
		dependencyTree, err = e.analyzeFrameworkDependencies(validPkgs, pkgsMetadata, fileToInfo, fset)
		if err != nil {
			logger.Printf("Warning: Failed to analyze framework dependencies: %v\n", err)
			e.reportPhase("framework-dependency analysis failed", time.Since(tDeps))
		} else {
			logger.Printf("Framework dependency analysis completed: %d packages found\n", dependencyTree.TotalPackages)
			e.reportPhase(fmt.Sprintf("framework dependencies analysed (%d pkgs)", dependencyTree.TotalPackages), time.Since(tDeps))

			// Auto-include framework packages in IncludePackages if requested
			if e.config.AutoIncludeFrameworkPackages {
				e.autoIncludeFrameworkPackages(dependencyTree, logger)

				// Re-filter packages to only include framework packages
				logger.Println("Re-filtering packages to include only framework packages...")
				pkgsMetadata, fileToInfo, importPaths = e.filterToFrameworkPackages(
					pkgsMetadata, fileToInfo, importPaths, dependencyTree)
				logger.Printf("Filtered to %d framework packages for metadata generation\n", len(pkgsMetadata))
			}
		}
	}

	// Generate metadata (now only on framework packages if auto-include is enabled)
	tMeta := time.Now()
	meta := metadata.GenerateMetadataWithLogger(pkgsMetadata, fileToInfo, importPaths, fset, logger, e.moduleImportPath())
	e.reportPhase(fmt.Sprintf("metadata generated (%d call edges, %d pkgs)", len(meta.CallGraph), len(meta.Packages)), time.Since(tMeta))
	if err := e.ctx().Err(); err != nil {
		return nil, err
	}

	// Resolved call graph (SSA+VTA) from the same loaded packages.
	if e.config.ResolveCallGraph {
		tResolved := time.Now()
		e.resolvedGraph = callgraph.Build(filteredPkgs)
		e.reportPhase(fmt.Sprintf("resolved call graph built (%d functions)", len(e.resolvedGraph.Graph.Nodes)), time.Since(tResolved))
		if err := e.ctx().Err(); err != nil {
			return nil, err
		}
	}

	// Store metadata in engine
	e.metadata = meta

	// Store framework dependency list in metadata (already analyzed above)
	if e.config.AnalyzeFrameworkDependencies && dependencyTree != nil {
		meta.FrameworkDependencyList = dependencyTree
	}

	return meta, nil
}

// defaultFrameworkConfig maps a detected framework name to its built-in
// config; unknown names (and "net/http") get the net/http config.
func defaultFrameworkConfig(framework string) *spec.APISpecConfig {
	switch framework {
	case "gin":
		return spec.DefaultGinConfig()
	case "chi":
		return spec.DefaultChiConfig()
	case "echo":
		return spec.DefaultEchoConfig()
	case "fiber":
		return spec.DefaultFiberConfig()
	case "mux":
		return spec.DefaultMuxConfig()
	default:
		return spec.DefaultHTTPConfig()
	}
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
			err = intspec.GeneratePaginatedCytoscapeHTML(meta, diagramPath, e.config.DiagramPageSize)
			if err != nil {
				return nil, fmt.Errorf("failed to generate paginated diagram: %w", err)
			}
		} else {
			// Use regular call graph visualization for smaller graphs
			err = intspec.GenerateCallGraphCytoscapeHTML(meta, diagramPath)
			if err != nil {
				return nil, fmt.Errorf("failed to generate diagram: %w", err)
			}
		}
	}

	// Framework dependency analysis is now handled in GenerateMetadataOnly()

	// Detect frameworks and load configuration. The first-seen framework is
	// the primary (whose Defaults/Info and unscoped helper patterns apply);
	// any further recognised frameworks merge in below as scoped views.
	detector := core.NewFrameworkDetector()
	frameworks, err := detector.DetectAll(e.config.moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to detect framework: %w", err)
	}
	framework := frameworks[0]

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
		apispecConfig = defaultFrameworkConfig(framework)
		// Additional recognised frameworks (a gin API next to a gorilla/mux
		// admin router, half-migrated projects): merge each one's
		// receiver-scoped view so its registrations are traced too. Scoped
		// patterns cannot claim another framework's calls, so the merge is
		// inert where the secondary framework is imported but not routing.
		for _, fw := range frameworks[1:] {
			apispecConfig = spec.MergeFrameworkConfigs(apispecConfig, spec.SecondaryView(defaultFrameworkConfig(fw)))
		}
		// Layer the stdlib net/http surface under the detected framework:
		// mixed projects (a framework API plus plain ServeMux ops endpoints
		// in one binary) are common, and net/http never appears in go.mod,
		// so import-based detection cannot pick it as a second framework.
		// Every merged pattern is receiver- or package-scoped, which keeps
		// the merge inert for pure-framework projects; user-supplied configs
		// (the branches above) are never augmented.
		if framework != "net/http" {
			apispecConfig = spec.MergeFrameworkConfigs(apispecConfig, spec.HTTPSecondaryConfig())
		}
	}

	// Merge built-in auth/security library presets based on the project's
	// imports (framework preset -> library presets -> user config; user wins).
	// The engine stays framework-agnostic: this only augments config data.
	intspec.ApplySecurityPresets(apispecConfig, meta)

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
	if err := e.ctx().Err(); err != nil {
		return nil, err
	}
	tTree := time.Now()
	var tree intspec.TrackerTreeInterface
	if e.config.UseLazyTracker {
		tree = intspec.NewLazyTree(meta, limits)
		e.reportPhase("tracker tree ready (lazy)", time.Since(tTree))
	} else {
		tree = intspec.NewTrackerTree(meta, limits, NewVerboseLogger(e.config.Verbose))
		e.reportPhase("tracker tree built", time.Since(tTree))
	}
	if err := e.ctx().Err(); err != nil {
		return nil, err
	}

	// Generate OpenAPI spec
	tSpec := time.Now()
	openAPISpec, secDiag, err := intspec.MapMetadataToOpenAPIWithDiagnostics(tree, apispecConfig, generatorConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAPI spec: %w", err)
	}
	if secDiag != nil {
		e.unresolvedSecurity = secDiag.UnresolvedMiddleware
		e.pathParamMismatches = secDiag.PathParamMismatches
	}
	e.reportPhase(fmt.Sprintf("spec mapped (%d paths)", len(openAPISpec.Paths)), time.Since(tSpec))

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

// applyConfigFilters folds the include/exclude patterns from the
// APISpecConfig (set via a config file or the UI) into the EngineConfig filter
// fields that shouldIncludePackage / shouldIncludeFile read. It unions with any
// CLI-provided patterns and de-duplicates, so it's safe to call more than once.
func (e *Engine) applyConfigFilters() {
	c := e.config.APISpecConfig
	if c == nil {
		return
	}
	e.config.IncludePackages = unionStrings(e.config.IncludePackages, c.Include.Packages)
	e.config.IncludeFiles = unionStrings(e.config.IncludeFiles, c.Include.Files)
	e.config.IncludeFunctions = unionStrings(e.config.IncludeFunctions, c.Include.Functions)
	e.config.IncludeTypes = unionStrings(e.config.IncludeTypes, c.Include.Types)
	e.config.ExcludePackages = unionStrings(e.config.ExcludePackages, c.Exclude.Packages)
	e.config.ExcludeFiles = unionStrings(e.config.ExcludeFiles, c.Exclude.Files)
	e.config.ExcludeFunctions = unionStrings(e.config.ExcludeFunctions, c.Exclude.Functions)
	e.config.ExcludeTypes = unionStrings(e.config.ExcludeTypes, c.Exclude.Types)
}

// unionStrings appends extras to base, skipping values already present, and
// preserves order.
func unionStrings(base, extras []string) []string {
	if len(extras) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, s := range base {
		seen[s] = struct{}{}
	}
	for _, s := range extras {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
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

// moduleImportPath reads the `module` path from go.mod at the resolved module
// root. This is the authoritative project import prefix; metadata generation
// uses it to classify project vs library packages (driving the Insight
// call-graph stats and external-vs-internal type resolution) instead of
// inferring it from import paths. Returns "" if go.mod is missing/unreadable.
func (e *Engine) moduleImportPath() string {
	if e.config.moduleRoot == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(e.config.moduleRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
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

// GetUnresolvedSecurity returns auth middleware detected during the most recent
// generation that matched no SecurityMapping (deduped). Empty when none.
func (e *Engine) GetUnresolvedSecurity() []intspec.MiddlewareRef {
	return e.unresolvedSecurity
}

// GetPathParamMismatches returns map-key path-variable reads (e.g.
// mux.Vars(r)["userId"]) from the most recent generation whose key matches no
// route placeholder — a likely typo. Empty when none.
func (e *Engine) GetPathParamMismatches() []intspec.PathParamMismatch {
	return e.pathParamMismatches
}

// SkippedPackages returns the in-module packages excluded from the most recent
// analysis because they failed to type-check. A non-empty result means the
// spec is likely incomplete — usually the project doesn't build (e.g. an
// unresolved/private dependency).
func (e *Engine) SkippedPackages() []SkippedPackage {
	return e.skipped
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
func (e *Engine) autoIncludeFrameworkPackages(frameworkList *metadata.FrameworkDependencyList, logger *VerboseLogger) {
	if frameworkList == nil || len(frameworkList.AllPackages) == 0 {
		return
	}

	logger.Println("Auto-including framework packages in IncludePackages...")

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

	logger.Printf("Added %d framework packages to IncludePackages\n", addedCount)
	logger.Printf("Total IncludePackages: %d\n", len(e.config.IncludePackages))

	// Print the added packages
	if addedCount > 0 {
		logger.Println("Added framework packages:")
		for _, dep := range frameworkList.AllPackages {
			if existingIncludes[dep.PackagePath] {
				frameworkType := dep.FrameworkType
				if dep.IsDirect {
					frameworkType += " (direct)"
				} else {
					frameworkType += " (indirect)"
				}
				logger.Printf("  - %s (%s)\n", dep.PackagePath, frameworkType)
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

	// keep decides whether a package survives the framework filter. Framework
	// packages are kept, and so is every in-module (project) package: dropping
	// project packages would discard interface implementations that are only
	// reached through dependency injection (e.g. a concrete store assigned to
	// an interface field), breaking interface→concrete resolution and type
	// inference. Only third-party non-framework deps are pruned.
	modPath := e.moduleImportPath()
	keep := func(pkgPath string) bool {
		if frameworkPackages[pkgPath] {
			return true
		}
		return modPath != "" && (pkgPath == modPath || strings.HasPrefix(pkgPath, modPath+"/"))
	}

	// Filter packages metadata
	filteredPkgsMetadata := make(map[string]map[string]*ast.File)
	for pkgPath, files := range pkgsMetadata {
		if keep(pkgPath) {
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
		if keep(pkgPath) {
			filteredImportPaths[fileName] = pkgPath
		}
	}

	return filteredPkgsMetadata, filteredFileToInfo, filteredImportPaths
}
