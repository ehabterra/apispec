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
	"golang.org/x/mod/modfile"
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
	// DefaultMaxInstancesPerKey mirrors the lazy tree's own default, so the
	// engine reports one number rather than two that can drift.
	DefaultMaxInstancesPerKey = intspec.DefaultMaxInstancesPerKey
	// DefaultMaxNodesPerRoute mirrors the lazy tree's own default.
	DefaultMaxNodesPerRoute = intspec.DefaultMaxNodesPerRoute
	DefaultMetadataFile     = "metadata.yaml"
	CopyrightNotice         = "apispec - Copyright 2026 Ehab Terra"
	LicenseNotice           = "Licensed under the Apache License 2.0. See LICENSE and NOTICE."
	FullLicenseNotice       = "\n\nCopyright 2026 Ehab Terra. Licensed under the Apache License 2.0. See LICENSE and NOTICE."
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
	// MaxInstancesPerKey bounds copies of one callee within an instance scope.
	// Zero uses DefaultMaxInstancesPerKey.
	MaxInstancesPerKey int
	// MaxNodesPerRoute bounds the nodes expanded below ONE route registration,
	// so a deep handler cannot consume the allowance the undiscovered routes
	// still need. Zero uses DefaultMaxNodesPerRoute. Lazy tracker only: the
	// eager tree bounds a route with MaxRecursionDepth instead and never reads
	// this.
	MaxNodesPerRoute int

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

	// detectedWrappers lists the project's own router types whose registration
	// patterns were derived from the parameter flow (issue #235).
	detectedWrappers []intspec.DetectedWrapper

	// expansionStats records how far tree expansion got during the last
	// generation, and whether the node budget cut it short (issue #233).
	expansionStats intspec.ExpansionStats

	// entrypointStats records what the entrypoint gate decided during the last
	// generation (issue #220): how many field-stored functions were found and how
	// many earned a root. Surfaced in the UI, where "0 rooted" is the difference
	// between "never looked" and "looked, nothing registers a route".
	entrypointStats intspec.EntrypointStats

	// unresolvedSecurity lists auth middleware detected during the last
	// generation that matched no SecurityMapping. Surfaced to callers (the UI)
	// so the user can map it to a scheme.
	unresolvedSecurity []intspec.MiddlewareRef

	// unresolvedRefs lists $refs the generated document could not satisfy and
	// that were repaired with a placeholder, from the most recent generation.
	unresolvedRefs []intspec.UnresolvedRef

	// pathParamMismatches lists map-key path-variable reads (mux.Vars(r)["x"])
	// whose key matches no route placeholder, gathered during the last generation.
	pathParamMismatches []intspec.PathParamMismatch

	// unresolvedPaths lists registrations left out of the last generation
	// because their path is built at runtime (issue #428).
	unresolvedPaths []intspec.UnresolvedPathRoute

	// routeDiscovery records what the route search had to work with and what it
	// found during the last generation (issue #379).
	routeDiscovery RouteDiscovery

	// resolvedGraph is the SSA+VTA resolved call graph, built during
	// GenerateMetadataOnly when config.ResolveCallGraph is set.
	resolvedGraph *callgraph.Resolved
}

// RouteDiscovery reports the inputs and the outcome of the route search.
//
// It exists because "zero paths" is ambiguous: a library with no HTTP routes
// legitimately documents nothing, and so does a project whose router apispec
// does not support, whose wiring style no pattern matched, or whose routing
// package the include/exclude filters removed. All four used to print
// "Successfully generated" and exit 0, which made them indistinguishable
// (issue #379).
//
// The signal that separates them is zero paths from a call graph that is NOT
// empty: there was code to walk, and nothing in it registered a route.
type RouteDiscovery struct {
	// CallEdges is the size of the call graph the route search walked.
	CallEdges int
	// Packages is how many packages that call graph came from.
	Packages int
	// Paths is how many paths the generated document ended up with.
	Paths int
	// Frameworks names the pattern sets in effect, primary first.
	Frameworks []string
}

// NothingMatched reports the condition worth telling the user about: code was
// analysed and no route registration in it matched any configured pattern.
func (d RouteDiscovery) NothingMatched() bool {
	return d.Paths == 0 && d.CallEdges > 0
}

// GetResolvedCallGraph returns the resolved call graph from the last
// generation, or nil when config.ResolveCallGraph was off.
func (e *Engine) GetResolvedCallGraph() *callgraph.Resolved {
	return e.resolvedGraph
}

// SkippedPackage is a package excluded from analysis because it did not load,
// with a representative reason.
type SkippedPackage struct {
	Package string `json:"package"`
	Reason  string `json:"reason"`
	// Kind separates "did not parse" from "did not type-check" because the fix
	// differs: a syntax error is always in the project's own source, while a
	// type error is as often a missing generated file or an unresolved private
	// dependency (issue #237). One of skipParse/skipType/skipLoad.
	Kind string `json:"kind,omitempty"`
}

// Why a package was excluded, in the words the report uses.
const (
	skipParse = "does not parse"
	skipType  = "does not type-check"
	skipLoad  = "could not be loaded"
)

// classifySkip reduces a package's load errors to one kind and one reason.
//
// A package that fails to parse reports SEVERAL errors — go/packages emits the
// driver's `# pkg` blob (ListError) alongside the parser's own message — so the
// kind cannot be read off the first one, and neither can a useful reason: the
// parser's message ("expected declaration, found ','") carries no position
// while the driver's blob does. So the kind comes from the strongest error
// present and the reason from the first one that names a file.
func classifySkip(errs []packages.Error) (kind, reason string) {
	kind = skipLoad
	for _, e := range errs {
		switch {
		case e.Kind == packages.ParseError:
			kind = skipParse
		case e.Kind == packages.TypeError && kind != skipParse:
			kind = skipType
		}
	}

	// Prefer an error that names a file: without a position the reason cannot
	// be acted on. The driver's blob leads with a "# import/path" line, which
	// only repeats what the report already prints.
	for _, e := range errs {
		if msg := trimDriverHeader(e.Msg); strings.Contains(msg, ".go:") {
			return kind, msg
		}
	}
	if len(errs) > 0 {
		return kind, trimDriverHeader(errs[0].Msg)
	}
	return kind, ""
}

// trimDriverHeader drops the leading "# import/path" line the go command puts
// in front of a build error, and folds the rest onto one line so a report stays
// one package per line.
func trimDriverHeader(msg string) string {
	lines := strings.Split(msg, "\n")
	if len(lines) > 1 && strings.HasPrefix(lines[0], "# ") {
		lines = lines[1:]
	}
	var kept []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "; ")
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
				kind, reason := classifySkip(pkg.Errors)
				e.skipped = append(e.skipped, SkippedPackage{Package: pkg.PkgPath, Reason: reason, Kind: kind})
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
	e.reportSkippedPackages()

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
			e.reportPhase(fmt.Sprintf("framework dependencies analysed (%d pkgs: %d direct, %d indirect)",
				dependencyTree.TotalPackages, dependencyTree.DirectPackages, dependencyTree.IndirectPackages), time.Since(tDeps))

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
	meta := metadata.GenerateMetadataWithLogger(pkgsMetadata, fileToInfo, importPaths, fset, logger, e.moduleImportPath(), e.config.moduleRoot)
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

// ComposeFrameworkConfig builds the effective framework config for a directory:
// the detected primary's config, every other detected framework merged in as a
// receiver-scoped view, and the stdlib net/http surface layered underneath. It
// returns the config and the detected frameworks in first-seen order.
//
// Exported because the UI must not compose this itself. It used to build a
// config from the *primary alone* — and a project whose primary is decided by
// file-walk order (issue #212) then loses every other framework's patterns: a
// single dummy file importing gorilla/mux inside photoprism makes mux primary,
// so a UI-built mux-only config documents zero of its gin routes while the CLI
// documents 107. One composition path, one answer.
func ComposeFrameworkConfig(dir string) (*spec.APISpecConfig, []string, error) {
	return ComposeFrameworkConfigWithPrimary(dir, "")
}

// ComposeFrameworkConfigWithPrimary is ComposeFrameworkConfig with the primary
// chosen by the caller — the UI's framework selector, where a user overrides
// what detection picked. Every OTHER detected framework still merges in as a
// scoped view, because overriding which framework leads must not silently
// discard the ones the project also uses.
func ComposeFrameworkConfigWithPrimary(dir, primary string) (*spec.APISpecConfig, []string, error) {
	frameworks, err := core.NewFrameworkDetector().DetectAll(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to detect framework: %w", err)
	}
	cfg, ordered := ComposeFrameworkConfigFrom(frameworks, primary)
	return cfg, ordered, nil
}

// ComposeFrameworkConfigFrom is the composition itself, for callers that have
// already detected the frameworks — detection parses the imports of every Go file
// in the project, so doing it twice for one run is pure waste.
func ComposeFrameworkConfigFrom(frameworks []string, primary string) (*spec.APISpecConfig, []string) {
	if len(frameworks) == 0 {
		frameworks = []string{core.StdlibFramework}
	}
	// Canonicalise before anything compares names. Detection always reports the
	// registry spelling, but an explicit primary is user input (the UI posts
	// req.Framework straight through), and a mixed-case "Gin" would otherwise
	// resolve to the gin config while failing every == comparison below —
	// duplicating the framework in the returned list and merging gin's own
	// patterns back over itself as a secondary.
	canonical := make([]string, len(frameworks))
	for i, fw := range frameworks {
		canonical[i], _ = core.CanonicalFrameworkName(fw)
	}
	frameworks = canonical
	primary, _ = core.CanonicalFrameworkName(primary)
	if primary == "" {
		primary = frameworks[0]
	} else {
		// The returned list always leads with the primary in effect — callers
		// (the UI's selector, its "detected" label) read frameworks[0] as the
		// one that leads. An explicit choice detection did not see (a house
		// router, or a framework used somewhere the walk missed) is added
		// rather than rejected; everything detected still merges under it.
		rest := make([]string, 0, len(frameworks))
		for _, fw := range frameworks {
			if fw != primary {
				rest = append(rest, fw)
			}
		}
		frameworks = append([]string{primary}, rest...)
	}

	cfg := spec.DefaultConfigForFramework(primary)
	// Additional recognised frameworks (a gin API next to a gorilla/mux admin
	// router, half-migrated projects): merge each one's receiver-scoped view so
	// its registrations are traced too. Scoped patterns cannot claim another
	// framework's calls, so the merge is inert where the secondary framework is
	// imported but not routing.
	for _, fw := range frameworks {
		if fw == primary {
			continue
		}
		cfg = spec.MergeFrameworkConfigs(cfg, spec.SecondaryView(spec.DefaultConfigForFramework(fw)))
	}
	// Layer the stdlib net/http surface under the detected framework: mixed
	// projects (a framework API plus plain ServeMux ops endpoints in one binary)
	// are common, and net/http never appears in go.mod, so import-based
	// detection cannot pick it as a second framework. Every merged pattern is
	// receiver- or package-scoped, which keeps the merge inert for
	// pure-framework projects.
	if primary != core.StdlibFramework {
		cfg = spec.MergeFrameworkConfigs(cfg, spec.HTTPSecondaryConfig())
	}
	return cfg, frameworks
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
	//
	// "First-seen" is file-walk order, so it is a property of the filenames
	// rather than of the API (issue #212). That is only tolerable because the
	// merge is symmetric for extraction: every framework's own patterns are
	// receiver-scoped (#211) and its ExternalTypes are carried, so a framework
	// documents the same surface whether or not it leads. What the primary
	// still decides is Info/Defaults — identical across the built-in presets
	// today. TestPrimaryFrameworkOrderInvariance pins the property, so a preset
	// that reintroduces a primary-only asymmetry fails there rather than in
	// someone's spec after a rename.
	frameworks, err := core.NewFrameworkDetector().DetectAll(e.config.moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to detect framework: %w", err)
	}
	framework := frameworks[0]
	if len(frameworks) > 1 {
		// Which framework led was previously invisible; if a primary-only
		// asymmetry ever does reach the output, this is what makes it
		// diagnosable instead of mysterious.
		NewVerboseLogger(e.config.Verbose).Printf("Frameworks detected: %v (primary: %s, merged as scoped views: %v)\n",
			frameworks, framework, frameworks[1:])
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
		// Composed the one way everything composes them — from the frameworks
		// detected above, so the import scan happens once per run.
		apispecConfig, _ = ComposeFrameworkConfigFrom(frameworks, "")
	}

	// Merge built-in auth/security library presets based on the project's
	// imports (framework preset -> library presets -> user config; user wins).
	// The engine stays framework-agnostic: this only augments config data.
	intspec.ApplySecurityPresets(apispecConfig, meta)

	// Entrypoint presets: which struct fields a command library dispatches
	// through (urfave/cli's Action, cobra's Run/RunE, …), keyed on the project's
	// imports. A function parked in such a field has no call edge from main, so
	// without this the route-registration subtree of a CLI-dispatched program is
	// unreachable and the project documents nothing (issue #220). Inert for a
	// project that imports none of them; a user config keeps precedence and can
	// declare its own field for a house dispatcher.
	intspec.ApplyEntrypointPresets(apispecConfig, meta)

	// A project's own router type registers nothing the framework's patterns can
	// see: the framework call sits inside the wrapper, where the path and handler
	// are the wrapper's parameters. Derive those patterns from that parameter flow
	// and apply the ones that resolved completely (issue #235). Inert for a project
	// that registers directly.
	e.detectedWrappers = intspec.DetectRouterWrappers(meta, apispecConfig)
	applyDetectedWrappers(apispecConfig, e.detectedWrappers)
	if e.config.Verbose && len(e.detectedWrappers) > 0 {
		NewVerboseLogger(e.config.Verbose).Printf("Router wrappers: %s\n", wrapperSummary(e.detectedWrappers))
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
		MaxInstancesPerKey: e.config.MaxInstancesPerKey,
		MaxNodesPerRoute:   e.config.MaxNodesPerRoute,
	}
	if err := e.ctx().Err(); err != nil {
		return nil, err
	}
	tTree := time.Now()
	var tree intspec.TrackerTreeInterface = intspec.NewLazyTree(meta, limits,
		intspec.WithHandlerInterfaceMethods(apispecConfig.Framework.HandlerInterfaceMethods),
		intspec.WithEntrypoints(apispecConfig.Framework.EntrypointPatterns,
			intspec.RouteRegistrationMatcher(apispecConfig, meta), NewVerboseLogger(e.config.Verbose)),
		intspec.WithTerminalRouteMatcher(intspec.TerminalRouteMatcher(apispecConfig, meta)))
	e.reportPhase("tracker tree ready", time.Since(tTree))
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
		e.unresolvedRefs = secDiag.UnresolvedRefs
		e.unresolvedPaths = secDiag.UnresolvedPaths
		e.reportUnresolvedRefs()
		e.reportUnresolvedPaths()
	}
	// Read after mapping: with the lazy tree the entrypoint gate runs during
	// expansion, which mapping is what triggers.
	if reporter, ok := tree.(interface {
		EntrypointStats() intspec.EntrypointStats
	}); ok {
		e.entrypointStats = reporter.EntrypointStats()
	}
	if reporter, ok := tree.(interface {
		ExpansionStats() intspec.ExpansionStats
	}); ok {
		e.expansionStats = reporter.ExpansionStats()
		if e.expansionStats.Truncated {
			// Louder than the tree's own stderr line: a truncated expansion means
			// the spec is incomplete, which is a result, not a debug detail.
			e.reportPhase(fmt.Sprintf("expansion truncated at the %d-node limit — the spec is incomplete", e.expansionStats.Limit), 0)
		}
		if n := e.expansionStats.RouteTruncations; n > 0 {
			// Local, unlike the whole-walk truncation above: these routes are
			// documented in less detail and no other route is affected (#264).
			e.reportPhase(fmt.Sprintf("per-route limit (%d) truncated %d of %d route subtrees — those routes are less detailed (first at %s)",
				e.expansionStats.RouteLimit, n, e.expansionStats.RoutesScoped, e.expansionStats.RouteFirstTruncated), 0)
		}
		if n := e.expansionStats.InstanceTruncations; n > 0 {
			// The other, quieter shortfall, reported separately because it is a
			// different claim: the walk may have finished the node budget fine and
			// still dropped call copies, which is how a response body goes missing
			// with nothing in the output to say so (issue #224). The scope is named
			// because it is what tells a bounded diamond from a starved route.
			e.reportPhase(fmt.Sprintf("instance cap (%d) dropped %d call copies — first: %s in scope %s",
				e.expansionStats.InstanceLimit, n, e.expansionStats.InstanceFirstKey, e.expansionStats.InstanceFirstScope), 0)
		}
	}
	e.reportPhase(fmt.Sprintf("spec mapped (%d paths)", len(openAPISpec.Paths)), time.Since(tSpec))

	e.routeDiscovery = RouteDiscovery{
		CallEdges:  len(meta.CallGraph),
		Packages:   len(meta.Packages),
		Paths:      len(openAPISpec.Paths),
		Frameworks: frameworks,
	}
	e.reportNoRoutes()

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
	// Parsed with the module-file grammar rather than by trimming a "module "
	// prefix: the hand-rolled version missed a tab-separated directive
	// entirely, and kept the trailing // comment or the surrounding quotes when
	// present. Each of those silently mis-answers "which packages are ours" —
	// an empty path reverts to the heuristics, and a path with a comment
	// glued on matches no package at all.
	return modfile.ModulePath(data)
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

// GetDetectedWrappers returns the router wrappers derived during the most recent
// generation, applied or not.
func (e *Engine) GetDetectedWrappers() []intspec.DetectedWrapper {
	return e.detectedWrappers
}

// GetExpansionStats returns how far tree expansion got during the most recent
// generation, including whether the node budget stopped it early.
func (e *Engine) GetExpansionStats() intspec.ExpansionStats {
	return e.expansionStats
}

// GetEntrypointStats returns what the entrypoint gate decided during the most
// recent generation. Zero when the project declares no entrypoints.
func (e *Engine) GetEntrypointStats() intspec.EntrypointStats {
	return e.entrypointStats
}

// reportUnresolvedRefs warns about references the document could not satisfy.
//
// Always-on rather than verbose-gated, like the truncation warnings: a
// reference with no component means a type resolved to nothing useful, so the
// operation is documented while its shape is not. It names the GO TYPE, which
// is what tells a user which dependency to register under externalTypes — the
// mangled component name does not.
func (e *Engine) reportUnresolvedRefs() {
	if len(e.unresolvedRefs) == 0 {
		return
	}

	var b strings.Builder
	sites := 0
	for i, ref := range e.unresolvedRefs {
		sites += ref.Sites
		if i > 0 {
			b.WriteString(", ")
		}
		if ref.GoType != "" {
			b.WriteString(ref.GoType)
		} else {
			b.WriteString(ref.Component)
		}
		if ref.Sites > 1 {
			fmt.Fprintf(&b, " (%d sites)", ref.Sites)
		}
	}

	e.reportPhase(fmt.Sprintf(
		"%d type(s) had no schema and were inlined as unresolved across %d reference(s): %s",
		len(e.unresolvedRefs), sites, b.String()), 0)
}

// reportUnresolvedPaths says so when a registration was found and understood
// except for its path, so it is missing from the document.
//
// Louder than the mapper's per-route line, and phrased as a count: one
// table-driven registration inside a loop is every route of that table, so the
// shortfall is usually the whole API rather than one endpoint.
func (e *Engine) reportUnresolvedPaths() {
	if len(e.unresolvedPaths) == 0 {
		return
	}
	first := e.unresolvedPaths[0]
	where := first.Position
	if where == "" {
		where = first.Handler
	}
	e.reportPhase(fmt.Sprintf(
		"%d registration(s) build their path at runtime and are NOT documented (first at %s) — "+
			"the routes they register are missing from the spec",
		len(e.unresolvedPaths), where), 0)
}

// reportNoRoutes says so when the walk analysed real code and matched no route
// registration in it — the case that used to print "Successfully generated"
// over an empty document and exit 0, indistinguishable from a project that
// genuinely serves no HTTP (issue #379).
//
// Written with log rather than reportPhase because it is not a phase: it is the
// result, and the " in 0s" a phase line appends reads as a timing on something
// that did not happen.
func (e *Engine) reportNoRoutes() {
	d := e.routeDiscovery
	if !d.NothingMatched() {
		return
	}
	// A registration that matched and was then dropped for an unreadable path is
	// a different diagnosis, and the advice below would send the reader looking
	// for an unsupported router they do not have (issue #428).
	if len(e.unresolvedPaths) > 0 {
		log.Printf("[engine] 0 paths documented: every registration that matched builds its path at runtime — see the %d line(s) above",
			len(e.unresolvedPaths))
		return
	}
	frameworks := strings.Join(d.Frameworks, ", ")
	if frameworks == "" {
		frameworks = "no framework"
	}
	log.Printf("[engine] no route registrations matched: 0 paths from %d call edges across %d package(s), with %s patterns in effect",
		d.CallEdges, d.Packages, frameworks)
	log.Printf("[engine] if this project serves HTTP, then its router is unsupported, is wired in a style no pattern matched, or was excluded by --include-*/--exclude-* filters — docs/DEBUGGING.md walks through telling those apart")
}

// maxSkippedPackagesReported bounds the per-package detail. One broken package
// takes every package that imports it down with it, so on a large project the
// list is a cascade of one root cause and printing all of it buries the line
// that matters.
const maxSkippedPackagesReported = 10

// reportSkippedPackages says so when the project's own packages did not load.
//
// This is the diagnostic that turns "apispec found nothing" into "your project
// does not compile". It was recorded and logged already, but only to the
// verbose logger, so a default run dropped a package's entire route tree and
// still printed "Successfully generated" over a thin spec (issue #237) — the
// same silence #379 removed for the no-routes case, and it is written the same
// way: log rather than reportPhase, because it is a result and not a phase.
func (e *Engine) reportSkippedPackages() {
	if len(e.skipped) == 0 {
		return
	}
	log.Printf("[engine] %d in-module package(s) were not analysed, so the spec is incomplete:", len(e.skipped))
	shown := e.skipped
	if len(shown) > maxSkippedPackagesReported {
		shown = shown[:maxSkippedPackagesReported]
	}
	parse := false
	for _, s := range shown {
		if s.Kind == skipParse {
			parse = true
		}
		log.Printf("[engine]   %s %s: %s", s.Package, s.Kind, s.Reason)
	}
	if rest := len(e.skipped) - len(shown); rest > 0 {
		log.Printf("[engine]   ... and %d more (one broken package takes its importers with it)", rest)
	}
	if parse {
		log.Printf("[engine] a package that does not parse is a syntax error in the project's own source — `go build ./...` reports the same")
	} else {
		log.Printf("[engine] check that the project builds (`go build ./...`); generated files and private dependencies are the usual causes")
	}
}

// GetUnresolvedRefs returns the references the most recent generation could not
// satisfy, after they were repaired. Non-empty means the spec loads but some
// operation's shape is a placeholder.
func (e *Engine) GetUnresolvedRefs() []intspec.UnresolvedRef {
	return e.unresolvedRefs
}

// GetUnresolvedPaths returns the registrations the most recent generation left
// undocumented because their path is built at runtime. Non-empty means the spec
// is missing endpoints that DO exist in the code — the count is the size of the
// gap, which a placeholder path used to hide.
func (e *Engine) GetUnresolvedPaths() []intspec.UnresolvedPathRoute {
	return e.unresolvedPaths
}

// GetRouteDiscovery returns what the route search walked and what it found in
// the most recent generation. NothingMatched() distinguishes "this project has
// no HTTP routes" from "apispec did not recognise this project's routes".
func (e *Engine) GetRouteDiscovery() RouteDiscovery {
	return e.routeDiscovery
}

// GetPathParamMismatches returns map-key path-variable reads (e.g.
// mux.Vars(r)["userId"]) from the most recent generation whose key matches no
// route placeholder — a likely typo. Empty when none.
func (e *Engine) GetPathParamMismatches() []intspec.PathParamMismatch {
	return e.pathParamMismatches
}

// SkippedPackages returns the in-module packages excluded from the most recent
// analysis because they failed to parse or type-check (Kind says which). A
// non-empty result means the spec is likely incomplete — usually the project
// doesn't build (e.g. a syntax error, a missing generated file, or an
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
	// The module path from go.mod decides which packages are the project's.
	// GenerateMetadataWithLogger has taken it since it was added; the dependency
	// analyser was left inferring the same answer from import-path shape, and
	// got it wrong for every domain-hosted module (issue #282).
	detector := metadata.NewFrameworkDetectorForModule(e.moduleImportPath())
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

// applyDetectedWrappers adds the derived patterns that resolved completely and
// returns how many were added.
//
// Incomplete derivations are deliberately left out: a pattern missing its path or
// handler produces a route with the wrong values rather than no route, which is
// worse than the project being undocumented (golden rule #7). They are still
// reported, so a user can see what was found and finish it by hand.
//
// Applying is idempotent. The config may be the CALLER's — the UI hands the same
// one to every run — so a second generation must not grow it a second copy of
// everything derived.
//
// A method can carry several roles at once (a context method that both writes a
// status and reads a parameter), so each role is considered on its own rather
// than as alternatives.
func applyDetectedWrappers(cfg *intspec.APISpecConfig, wrappers []intspec.DetectedWrapper) int {
	applied := 0
	for _, w := range wrappers {
		if !w.Complete {
			continue
		}
		roled := false
		if w.Mount != nil {
			roled = true
			if addPatternOnce(&cfg.Framework.MountPatterns, *w.Mount, func(p intspec.MountPattern) (string, string) {
				return p.CallRegex, p.RecvTypeRegex
			}) {
				applied++
			}
		}
		// A response derivation with neither role resolved describes nothing.
		if w.Response != nil && (w.Response.TypeFromArg || w.Response.StatusFromArg) {
			roled = true
			if addPatternOnce(&cfg.Framework.ResponsePatterns, *w.Response, func(p intspec.ResponsePattern) (string, string) {
				return p.CallRegex, p.RecvTypeRegex
			}) {
				applied++
			}
		}
		if w.Request != nil {
			roled = true
			if addPatternOnce(&cfg.Framework.RequestBodyPatterns, *w.Request, func(p intspec.RequestBodyPattern) (string, string) {
				return p.CallRegex, p.RecvTypeRegex
			}) {
				applied++
			}
		}
		if w.Param != nil {
			roled = true
			if addPatternOnce(&cfg.Framework.ParamPatterns, *w.Param, func(p intspec.ParamPattern) (string, string) {
				return p.CallRegex, p.RecvTypeRegex
			}) {
				applied++
			}
		}
		if !roled {
			if addPatternOnce(&cfg.Framework.RoutePatterns, w.Pattern, func(p intspec.RoutePattern) (string, string) {
				return p.CallRegex, p.RecvTypeRegex
			}) {
				applied++
			}
		}
	}
	return applied
}

// addPatternOnce prepends a derived pattern unless one matching the same calls on
// the same receiver is already configured — which is what makes a second run on
// the same config produce the same config.
func addPatternOnce[T any](patterns *[]T, add T, identity func(T) (string, string)) bool {
	call, recv := identity(add)
	for _, existing := range *patterns {
		if c, r := identity(existing); c == call && r == recv {
			return false
		}
	}
	*patterns = append([]T{add}, (*patterns)...)
	return true
}

// wrapperSummary renders what was derived, for the run's log line.
func wrapperSummary(wrappers []intspec.DetectedWrapper) string {
	var parts []string
	for _, w := range wrappers {
		state := "applied"
		if !w.Complete {
			state = "incomplete, not applied"
		}
		kind := "route"
		switch {
		case w.Mount != nil:
			kind = "mount"
		case w.Response != nil:
			kind = "response"
		case w.Request != nil:
			kind = "request"
		case w.Param != nil:
			kind = "param"
		}
		parts = append(parts, fmt.Sprintf("%s %v %s via %s (%s)", w.RecvType, w.Methods, kind, w.Via, state))
	}
	return strings.Join(parts, "; ")
}
