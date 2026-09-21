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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/profiler"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
	"gopkg.in/yaml.v3"
)

// stringSliceFlag implements flag.Value for string slices
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// strictFlag implements --strict, which is both a switch and a list: `--strict`
// enables every quality gate, `--strict=security,paths` only those.
//
// IsBoolFlag is what makes the bare form legal — without it the flag package
// would swallow the next argument as the value, so `apispec --strict ./api`
// would set the categories to "./api" and lose the directory. The cost is that
// the value MUST be attached with `=`, which is the same rule Go's own bool
// flags follow, and the help text says so.
type strictFlag struct {
	enabled    bool
	categories []engine.StrictCategory
}

func (s *strictFlag) String() string {
	if !s.enabled {
		return ""
	}
	names := make([]string, 0, len(s.categories))
	for _, c := range s.categories {
		names = append(names, string(c))
	}
	return strings.Join(names, ",")
}

func (s *strictFlag) Set(value string) error {
	// `--strict=false` turns the gate off, so a wrapper script can neutralise a
	// --strict baked into a Makefile without editing it.
	if strings.EqualFold(value, "false") {
		s.enabled, s.categories = false, nil
		return nil
	}
	if strings.EqualFold(value, "true") {
		value = ""
	}
	categories, err := engine.ParseStrictCategories(value)
	if err != nil {
		return err
	}
	s.enabled, s.categories = true, categories
	return nil
}

func (s *strictFlag) IsBoolFlag() bool { return true }

// Version info - can be injected at build time via -ldflags or detected at runtime
var (
	Version   = "0.0.1" // Default version, overridden by -ldflags or runtime detection
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// detectVersionInfo attempts to detect version information at runtime
func detectVersionInfo() {
	// If version info was already injected via -ldflags, don't override it
	if Version != "0.0.1" {
		return
	}

	// Try to get build info from runtime/debug
	if info, ok := debug.ReadBuildInfo(); ok {
		// Always get Go version from build info
		if info.GoVersion != "" {
			GoVersion = info.GoVersion
		}

		// Get version from build info (usually the module version or VCS tag)
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}

		// Extract commit, build time, and other VCS info from build settings
		hasVCSInfo := false
		isModified := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				hasVCSInfo = true
				if len(setting.Value) >= 7 {
					Commit = setting.Value[:7] // Short commit hash
				} else {
					Commit = setting.Value
				}
			case "vcs.time":
				hasVCSInfo = true
				BuildDate = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					isModified = true
				}
			}
		}

		// Add dirty flag if modified (but only if we don't already have it)
		if isModified && !strings.Contains(Version, "+dirty") {
			Version += "+dirty"
		}

		// If we have VCS info but no version, we're likely in development
		if hasVCSInfo && (Version == "0.0.1" || Version == "(devel)") {
			Version = "dev"
		}
	}

	// Final fallback - if we still don't have meaningful info
	if Version == "0.0.1" || Version == "(devel)" {
		// This happens when installed via go install without VCS info
		// Try to at least show that it's a go install version
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path == "github.com/ehabterra/apispec" {
			Version = "latest (go install)"
		} else {
			Version = "unknown (go install)"
		}
	}
}

func printVersion() {
	// Detect version info if not already set via -ldflags
	detectVersionInfo()

	fmt.Printf("apispec version: %s\n", Version)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Printf("Build date: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
	fmt.Println(engine.CopyrightNotice)
	fmt.Println(engine.LicenseNotice)
}

// CLIConfig holds the configuration parsed from command line arguments
type CLIConfig struct {
	Verbose                      bool
	InputDir                     string
	OutputFile                   string
	Title                        string
	APIVersion                   string
	Description                  string
	TermsOfService               string
	ContactName                  string
	ContactURL                   string
	ContactEmail                 string
	LicenseName                  string
	LicenseURL                   string
	OpenAPIVersion               string
	OperationIDNaming            string
	SchemaNaming                 string
	ConfigFile                   string
	OutputConfig                 string
	WriteMetadata                bool
	SplitMetadata                bool
	DiagramPath                  string
	PaginatedDiagram             bool
	DiagramPageSize              int
	MaxNodesPerTree              int
	MaxChildrenPerNode           int
	MaxArgsPerFunction           int
	MaxNestedArgsDepth           int
	MaxRecursionDepth            int
	MaxInstancesPerKey           int
	MaxResponseInstancesPerKey   int
	MaxNodesPerRoute             int
	ShowVersion                  bool
	OutputFlagSet                bool
	Strict                       strictFlag
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
	AutoExcludeTests             bool
	AutoExcludeMocks             bool
	// Profiling options
	CPUProfile         bool
	MemProfile         bool
	BlockProfile       bool
	MutexProfile       bool
	TraceProfile       bool
	CustomMetrics      bool
	ProfileOutputDir   string
	ProfileCPUPath     string
	ProfileMemPath     string
	ProfileBlockPath   string
	ProfileMutexPath   string
	ProfileTracePath   string
	ProfileMetricsPath string
}

// parseFlags parses command line arguments and returns a CLIConfig
func parseFlags(args []string) (*CLIConfig, error) {
	// Create a new flag set to avoid global state
	fs := flag.NewFlagSet("apispec", flag.ContinueOnError)

	config := &CLIConfig{}

	// Version flag
	fs.BoolVar(&config.ShowVersion, "version", false, "Show version information")
	fs.BoolVar(&config.ShowVersion, "V", false, "Shorthand for --version")

	// Custom help
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n%s\n\nUsage: %s [flags]\n\nFlags:\n",
			engine.CopyrightNotice, engine.LicenseNotice, os.Args[0])
		fs.PrintDefaults()
		fmt.Printf("\nExamples:\n")
		fmt.Printf("  %s -o spec.yaml -d ./api\n", os.Args[0])
		fmt.Printf("  %s -o spec.yaml -d ./api --diagram diagram.html\n", os.Args[0])
		fmt.Printf("  %s -o spec.yaml -d ./api --diagram diagram.html --diagram-page-size 50\n", os.Args[0])
		fmt.Printf("  %s -o spec.yaml -d ./api --diagram diagram.html --paginated-diagram\n", os.Args[0])
		fmt.Printf("\nPerformance Tips:\n")
		fmt.Printf("  • Use --paginated-diagram for large call graphs (1000+ edges)\n")
		fmt.Printf("  • Use --diagram-page-size 50 for very large graphs (3000+ edges)\n")
	}

	// Parse flags
	fs.StringVar(&config.InputDir, "dir", engine.DefaultInputDir, "Input directory containing Go source files")
	fs.StringVar(&config.InputDir, "d", engine.DefaultInputDir, "Shorthand for --dir")

	fs.StringVar(&config.OutputFile, "output", engine.DefaultOutputFile, "Output file path")
	fs.StringVar(&config.OutputFile, "o", engine.DefaultOutputFile, "Shorthand for --output")

	fs.StringVar(&config.Title, "title", engine.DefaultTitle, "API title")
	fs.StringVar(&config.Title, "t", engine.DefaultTitle, "Shorthand for --title")

	fs.StringVar(&config.APIVersion, "api-version", engine.DefaultAPIVersion, "API version")
	fs.StringVar(&config.APIVersion, "v", engine.DefaultAPIVersion, "Shorthand for --api-version")

	fs.StringVar(&config.Description, "description", "", "API description")
	fs.StringVar(&config.Description, "D", "", "Shorthand for --description")

	fs.StringVar(&config.TermsOfService, "terms", "", "Terms of service URL")
	fs.StringVar(&config.TermsOfService, "T", "", "Shorthand for --terms")

	fs.StringVar(&config.ContactName, "contact-name", engine.DefaultContactName, "Contact name")
	fs.StringVar(&config.ContactName, "N", engine.DefaultContactName, "Shorthand for --contact-name")

	fs.StringVar(&config.ContactURL, "contact-url", engine.DefaultContactURL, "Contact URL")
	fs.StringVar(&config.ContactURL, "U", engine.DefaultContactURL, "Shorthand for --contact-url")

	fs.StringVar(&config.ContactEmail, "contact-email", engine.DefaultContactEmail, "Contact email")
	fs.StringVar(&config.ContactEmail, "E", engine.DefaultContactEmail, "Shorthand for --contact-email")

	fs.StringVar(&config.LicenseName, "license-name", "", "License name")
	fs.StringVar(&config.LicenseName, "L", "", "Shorthand for --license-name")

	fs.StringVar(&config.LicenseURL, "license-url", "", "License URL")
	fs.StringVar(&config.LicenseURL, "lu", "", "Shorthand for --license-url")

	fs.StringVar(&config.OpenAPIVersion, "openapi-version", engine.DefaultOpenAPIVersion, "OpenAPI specification version")
	fs.StringVar(&config.OpenAPIVersion, "O", engine.DefaultOpenAPIVersion, "Shorthand for --openapi-version")
	fs.StringVar(&config.OperationIDNaming, "operation-id", "", "operationId style: full (the Go symbol, default), receiver-method (e.g. userHandler.list), or method-path (e.g. getUsersById); overrides naming.operationId in the config")
	fs.StringVar(&config.SchemaNaming, "schema-names", "", "component name style: full (package-qualified, default) or short (the type name, qualified only where names collide); overrides naming.schemaNames in the config")

	fs.StringVar(&config.ConfigFile, "config", "", "Configuration file path")
	fs.StringVar(&config.ConfigFile, "c", "", "Shorthand for --config")

	fs.StringVar(&config.OutputConfig, "output-config", "", "Output effective configuration to file")
	fs.StringVar(&config.OutputConfig, "oc", "", "Shorthand for --output-config")

	fs.BoolVar(&config.WriteMetadata, "write-metadata", false, "Write metadata to file")
	fs.BoolVar(&config.WriteMetadata, "w", false, "Shorthand for --write-metadata")

	fs.BoolVar(&config.SplitMetadata, "split-metadata", false, "Write split metadata files")
	fs.BoolVar(&config.SplitMetadata, "s", false, "Shorthand for --split-metadata")

	fs.StringVar(&config.DiagramPath, "diagram", "", "Generate call graph diagram")
	fs.StringVar(&config.DiagramPath, "g", "", "Shorthand for --diagram")

	fs.BoolVar(&config.PaginatedDiagram, "paginated-diagram", false, "Use paginated diagram for better performance with large call graphs")
	fs.BoolVar(&config.PaginatedDiagram, "pd", false, "Shorthand for --paginated-diagram")

	fs.IntVar(&config.DiagramPageSize, "diagram-page-size", 100, "Number of nodes per page in paginated diagram (50-500)")
	fs.IntVar(&config.DiagramPageSize, "dps", 100, "Shorthand for --diagram-page-size")

	fs.IntVar(&config.MaxNodesPerTree, "max-nodes", engine.DefaultMaxNodesPerTree,
		"Maximum nodes in the walk that finds route registrations (eager tracker: nodes per tree). Spending it costs whole routes; see --max-nodes-per-route for a route's detail")
	fs.IntVar(&config.MaxNodesPerTree, "mn", engine.DefaultMaxNodesPerTree, "Shorthand for --max-nodes")

	fs.IntVar(&config.MaxChildrenPerNode, "max-children", engine.DefaultMaxChildrenPerNode, "Maximum children per node")
	fs.IntVar(&config.MaxChildrenPerNode, "mc", engine.DefaultMaxChildrenPerNode, "Shorthand for --max-children")

	fs.IntVar(&config.MaxArgsPerFunction, "max-args", engine.DefaultMaxArgsPerFunction, "Maximum arguments per function")
	fs.IntVar(&config.MaxArgsPerFunction, "ma", engine.DefaultMaxArgsPerFunction, "Shorthand for --max-args")

	fs.IntVar(&config.MaxNestedArgsDepth, "max-nested-args", engine.DefaultMaxNestedArgsDepth, "Maximum nested arguments depth")
	fs.IntVar(&config.MaxNestedArgsDepth, "md", engine.DefaultMaxNestedArgsDepth, "Shorthand for --max-nested-args")

	fs.IntVar(&config.MaxRecursionDepth, "max-recursion-depth", engine.DefaultMaxRecursionDepth, "Maximum recursion depth to prevent infinite loops")
	fs.IntVar(&config.MaxRecursionDepth, "mrd", engine.DefaultMaxRecursionDepth, "Shorthand for --max-recursion-depth")
	fs.IntVar(&config.MaxInstancesPerKey, "max-instances-per-key", engine.DefaultMaxInstancesPerKey,
		"Maximum copies of one callee within an instance scope — the bound on call diamonds")
	fs.IntVar(&config.MaxResponseInstancesPerKey, "max-response-instances-per-key", engine.DefaultMaxResponseInstancesPerKey,
		"Maximum copies of a call a response pattern is looking for, within an instance scope. Higher than the general bound: these are the calls a route's body is resolved from, and a shared responder needs one copy per route that reaches it")
	fs.IntVar(&config.MaxNodesPerRoute, "max-nodes-per-route", engine.DefaultMaxNodesPerRoute,
		"Maximum tree nodes expanded below one route registration, bounding a route's detail without starving the others")

	// Include/exclude flags
	fs.Var((*stringSliceFlag)(&config.IncludeFiles), "include-file", "Include files matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.IncludePackages), "include-package", "Include packages matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.IncludeFunctions), "include-function", "Include functions matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.IncludeTypes), "include-type", "Include types matching pattern (can be specified multiple times)")

	fs.Var((*stringSliceFlag)(&config.ExcludeFiles), "exclude-file", "Exclude files matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.ExcludePackages), "exclude-package", "Exclude packages matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.ExcludeFunctions), "exclude-function", "Exclude functions matching pattern (can be specified multiple times)")
	fs.Var((*stringSliceFlag)(&config.ExcludeTypes), "exclude-type", "Exclude types matching pattern (can be specified multiple times)")

	fs.BoolVar(&config.SkipCGOPackages, "skip-cgo", true, "Skip packages with CGO dependencies that may cause build errors")

	// Profiling flags
	fs.BoolVar(&config.CPUProfile, "cpu-profile", false, "Enable CPU profiling")
	fs.BoolVar(&config.MemProfile, "mem-profile", false, "Enable memory profiling")
	fs.BoolVar(&config.BlockProfile, "block-profile", false, "Enable block profiling")
	fs.BoolVar(&config.MutexProfile, "mutex-profile", false, "Enable mutex profiling")
	fs.BoolVar(&config.TraceProfile, "trace-profile", false, "Enable trace profiling")
	fs.BoolVar(&config.CustomMetrics, "custom-metrics", false, "Enable custom metrics collection")

	fs.StringVar(&config.ProfileOutputDir, "profile-dir", "profiles", "Directory for profiling output files")
	fs.StringVar(&config.ProfileCPUPath, "cpu-profile-path", "cpu.prof", "CPU profile output file")
	fs.StringVar(&config.ProfileMemPath, "mem-profile-path", "mem.prof", "Memory profile output file")
	fs.StringVar(&config.ProfileBlockPath, "block-profile-path", "block.prof", "Block profile output file")
	fs.StringVar(&config.ProfileMutexPath, "mutex-profile-path", "mutex.prof", "Mutex profile output file")
	fs.StringVar(&config.ProfileTracePath, "trace-profile-path", "trace.out", "Trace profile output file")
	fs.StringVar(&config.ProfileMetricsPath, "metrics-path", "metrics.json", "Custom metrics output file")

	fs.BoolVar(&config.AnalyzeFrameworkDependencies, "analyze-framework-dependencies", true, "Analyze framework dependencies")
	fs.BoolVar(&config.AnalyzeFrameworkDependencies, "afd", true, "Shorthand for --analyze-framework-dependencies")

	fs.BoolVar(&config.AutoIncludeFrameworkPackages, "auto-include-framework-packages", true, "Auto-include framework packages")
	fs.BoolVar(&config.AutoIncludeFrameworkPackages, "aifp", true, "Shorthand for --auto-include-framework-packages")

	fs.BoolVar(&config.AutoExcludeTests, "auto-exclude-tests", true, "Auto-exclude test files")
	fs.BoolVar(&config.AutoExcludeTests, "aet", true, "Shorthand for --auto-exclude-tests")

	fs.BoolVar(&config.AutoExcludeMocks, "auto-exclude-mocks", true, "Auto-exclude mock files")
	fs.BoolVar(&config.AutoExcludeMocks, "aem", true, "Shorthand for --auto-exclude-mocks")

	// No backquotes in this usage string: PrintDefaults reads a backquoted word
	// as the flag's value name.
	fs.Var(&config.Strict, "strict",
		"Exit "+strconv.Itoa(strictExitCode)+" when the spec came out incomplete, instead of exiting 0. "+
			"Bare --strict gates on everything; --strict=<categories> (comma-separated, attached with =) gates on "+
			strings.Join(engine.StrictCategoryNames(), ", ")+" only")

	// Verbose output control
	fs.BoolVar(&config.Verbose, "verbose", false, "Enable verbose output")
	fs.BoolVar(&config.Verbose, "vb", false, "Shorthand for --verbose")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	// A misspelt style would otherwise fall back to "full" with only a log
	// line, and the document would look as if the flag had been ignored.
	if err := validateNaming("--operation-id", config.OperationIDNaming,
		intspec.NamingFull, intspec.NamingReceiverMethod, intspec.NamingMethodPath); err != nil {
		return nil, err
	}
	if err := validateNaming("--schema-names", config.SchemaNaming,
		intspec.NamingFull, intspec.NamingShort); err != nil {
		return nil, err
	}

	// Handle positional arguments (override --dir flag)
	if len(fs.Args()) > 0 {
		config.InputDir = fs.Args()[0]
	}

	// Check if output flag was explicitly set
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "output" || f.Name == "o" {
			config.OutputFlagSet = true
		}
	})

	// Validate diagram page size
	if config.DiagramPageSize < 50 {
		config.DiagramPageSize = 50
	} else if config.DiagramPageSize > 500 {
		config.DiagramPageSize = 500
	}

	return config, nil
}

// runGeneration generates the OpenAPI specification and returns the spec object directly (like metadata)
func runGeneration(config *CLIConfig) (*spec.OpenAPISpec, *engine.Engine, error) {
	// Create engine configuration
	engineConfig := &engine.EngineConfig{
		InputDir:                     config.InputDir,
		OutputFile:                   config.OutputFile,
		Title:                        config.Title,
		APIVersion:                   config.APIVersion,
		Description:                  config.Description,
		TermsOfService:               config.TermsOfService,
		ContactName:                  config.ContactName,
		ContactURL:                   config.ContactURL,
		ContactEmail:                 config.ContactEmail,
		LicenseName:                  config.LicenseName,
		LicenseURL:                   config.LicenseURL,
		OpenAPIVersion:               config.OpenAPIVersion,
		OperationIDNaming:            config.OperationIDNaming,
		SchemaNaming:                 config.SchemaNaming,
		ConfigFile:                   config.ConfigFile,
		OutputConfig:                 config.OutputConfig,
		WriteMetadata:                config.WriteMetadata,
		SplitMetadata:                config.SplitMetadata,
		DiagramPath:                  config.DiagramPath,
		PaginatedDiagram:             config.PaginatedDiagram,
		DiagramPageSize:              config.DiagramPageSize,
		MaxNodesPerTree:              config.MaxNodesPerTree,
		MaxChildrenPerNode:           config.MaxChildrenPerNode,
		MaxArgsPerFunction:           config.MaxArgsPerFunction,
		MaxNestedArgsDepth:           config.MaxNestedArgsDepth,
		MaxRecursionDepth:            config.MaxRecursionDepth,
		MaxInstancesPerKey:           config.MaxInstancesPerKey,
		MaxResponseInstancesPerKey:   config.MaxResponseInstancesPerKey,
		MaxNodesPerRoute:             config.MaxNodesPerRoute,
		IncludeFiles:                 config.IncludeFiles,
		IncludePackages:              config.IncludePackages,
		IncludeFunctions:             config.IncludeFunctions,
		IncludeTypes:                 config.IncludeTypes,
		ExcludeFiles:                 config.ExcludeFiles,
		ExcludePackages:              config.ExcludePackages,
		ExcludeFunctions:             config.ExcludeFunctions,
		ExcludeTypes:                 config.ExcludeTypes,
		SkipCGOPackages:              config.SkipCGOPackages,
		AnalyzeFrameworkDependencies: config.AnalyzeFrameworkDependencies,
		AutoIncludeFrameworkPackages: config.AutoIncludeFrameworkPackages,
		AutoExcludeTests:             config.AutoExcludeTests,
		AutoExcludeMocks:             config.AutoExcludeMocks,
		Verbose:                      config.Verbose,
	}

	// Create engine and generate OpenAPI spec
	genEngine := engine.NewEngine(engineConfig)
	openAPISpec, err := genEngine.GenerateOpenAPI()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate OpenAPI spec: %w", err)
	}

	return openAPISpec, genEngine, nil
}

// runGenerationWithProfiling generates the OpenAPI specification with profiling support
func runGenerationWithProfiling(config *CLIConfig, prof *profiler.Profiler) (*spec.OpenAPISpec, *engine.Engine, error) {
	if prof == nil || prof.GetMetrics() == nil {
		return runGeneration(config)
	}

	mc := prof.GetMetrics()

	// Profile the entire generation process
	var openAPISpec *spec.OpenAPISpec
	var genEngine *engine.Engine

	err := profiler.ProfileFunc(mc, "openapi_generation", func() error {
		var genErr error
		openAPISpec, genEngine, genErr = runGeneration(config)
		return genErr
	}, map[string]string{"operation": "generation"})

	if err != nil {
		return nil, nil, err
	}

	// Record additional metrics
	mc.SetGauge("generation.success", 1, "count", map[string]string{"operation": "generation"})

	return openAPISpec, genEngine, nil
}

// generatePerformanceAnalysis generates a performance analysis report
func generatePerformanceAnalysis(prof *profiler.Profiler, config *CLIConfig) error {
	mc := prof.GetMetrics()
	if mc == nil {
		return nil
	}

	// Write metrics to file
	metricsPath := filepath.Join(config.ProfileOutputDir, config.ProfileMetricsPath)
	if err := mc.WriteToFile(metricsPath); err != nil {
		return fmt.Errorf("failed to write metrics: %w", err)
	}

	// Analyze metrics
	analyzer := profiler.NewPerformanceAnalyzer()
	metrics := mc.GetMetrics()
	report := analyzer.AnalyzeMetrics(metrics)

	// Log basic report info
	if config.Verbose {
		fmt.Printf("Performance Analysis: %d issues found\n", report.TotalIssues)
		if report.TotalIssues > 0 {
			fmt.Printf("Issues by severity: %+v\n", report.Summary)
		}
	}

	return nil
}

// writeOutput writes OpenAPI spec directly to file using streaming encoder (like metadata)
func writeOutput(openAPISpec interface{}, config *CLIConfig, genEngine *engine.Engine) error {
	// If output is the default (openapi.json) and no explicit output flag was set, output to stdout
	if config.OutputFile == engine.DefaultOutputFile && !config.OutputFlagSet {
		ext := strings.ToLower(filepath.Ext("openapi.json"))
		if ext == ".yaml" || ext == ".yml" {
			encoder := yaml.NewEncoder(os.Stdout)
			encoder.SetIndent(2)
			if err := encoder.Encode(openAPISpec); err != nil {
				err = encoder.Close()
				if err != nil {
					return fmt.Errorf("failed to close YAML encoder: %w", err)
				}
				return fmt.Errorf("failed to encode OpenAPI spec to YAML: %w", err)
			}
			return encoder.Close()
		} else {
			data, err := json.MarshalIndent(openAPISpec, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal OpenAPI spec to JSON: %w", err)
			}
			fmt.Print(string(data))
			return nil
		}
	} else {
		// Relative --output paths land in the analyzed module (so
		// `apispec --dir proj -o spec.yaml` writes proj/spec.yaml);
		// absolute paths must be honored as given, not joined under it.
		outputPath := config.OutputFile
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(genEngine.ModuleRoot(), outputPath)
		}

		// Create file
		file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() {
			err = file.Close()
			if err != nil {
				log.Printf("Failed to close file: %v", err)
			}
		}()

		ext := strings.ToLower(filepath.Ext(config.OutputFile))
		if ext == ".yaml" || ext == ".yml" {
			// Use direct file writing like metadata (no memory buffering)
			encoder := yaml.NewEncoder(file)
			encoder.SetIndent(2)

			if err := encoder.Encode(openAPISpec); err != nil {
				err = encoder.Close()
				if err != nil {
					return fmt.Errorf("failed to close YAML encoder: %w", err)
				}
				return fmt.Errorf("failed to encode OpenAPI spec to YAML: %w", err)
			}

			if err := encoder.Close(); err != nil {
				return fmt.Errorf("failed to close YAML encoder: %w", err)
			}
		} else {
			data, err := json.MarshalIndent(openAPISpec, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal OpenAPI spec to JSON: %w", err)
			}
			if _, err := file.Write(data); err != nil {
				return fmt.Errorf("failed to write JSON data: %w", err)
			}
		}

		// "Successfully generated" over a document with no paths is what made an
		// unmatched router look like a working run (issue #379). The engine has
		// already said why on stderr; this line must not contradict it —
		// including when something DID match and was dropped for an unreadable
		// path, which is not the same finding (issue #428).
		switch {
		case genEngine.GetRouteDiscovery().Paths > 0:
			fmt.Println("Successfully generated:", outputPath)
		case len(genEngine.GetUnresolvedPaths()) > 0:
			fmt.Println("Generated (0 paths — every registration builds its path at runtime):", outputPath)
		default:
			fmt.Println("Generated (0 paths — nothing matched):", outputPath)
		}
	}
	return nil
}

func main() {
	start := time.Now()
	// Print copyright and license info at the very start
	fmt.Println(engine.CopyrightNotice)

	// Parse command line arguments
	config, err := parseFlags(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatalf("Failed to parse flags: %v", err)
	}

	// Handle version flag early
	if config.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	// Initialize profiling if enabled
	var prof *profiler.Profiler
	if config.CPUProfile || config.MemProfile || config.BlockProfile ||
		config.MutexProfile || config.TraceProfile || config.CustomMetrics {
		profConfig := &profiler.ProfilerConfig{
			CPUProfile:       config.CPUProfile,
			CPUProfilePath:   config.ProfileCPUPath,
			MemProfile:       config.MemProfile,
			MemProfilePath:   config.ProfileMemPath,
			BlockProfile:     config.BlockProfile,
			BlockProfilePath: config.ProfileBlockPath,
			MutexProfile:     config.MutexProfile,
			MutexProfilePath: config.ProfileMutexPath,
			TraceProfile:     config.TraceProfile,
			TraceProfilePath: config.ProfileTracePath,
			CustomMetrics:    config.CustomMetrics,
			MetricsPath:      config.ProfileMetricsPath,
			OutputDir:        config.ProfileOutputDir,
		}

		prof = profiler.NewProfiler(profConfig)
		if err := prof.Start(); err != nil {
			log.Fatalf("Failed to start profiling: %v", err)
		}
		defer func() {
			if err := prof.Stop(); err != nil {
				log.Printf("Failed to stop profiling: %v", err)
			}
		}()
	}

	// Generate OpenAPI specification with profiling
	openAPISpec, genEngine, err := runGenerationWithProfiling(config, prof)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Write output directly (like metadata) to avoid memory buffering
	if err := writeOutput(openAPISpec, config, genEngine); err != nil {
		log.Fatalf("%v", err)
	}

	// Generate performance analysis if custom metrics are enabled
	if prof != nil && prof.GetMetrics() != nil {
		if err := generatePerformanceAnalysis(prof, config); err != nil {
			log.Printf("Failed to generate performance analysis: %v", err)
		}
	}

	fmt.Printf("Time elapsed: %s\n", time.Since(start))

	// Last, after the document has been written: --strict decides an exit code,
	// never a document. The spec a strict run produces is byte-identical to the
	// one a normal run produces, and it is still written when the gate fails —
	// a CI job that fails the gate should still be able to publish the artifact
	// and diff it, which is how anyone works out what was lost.
	if code := strictExit(config, genEngine); code != 0 {
		// The deferred profiler stop does not run past os.Exit, so unwind it
		// here; nothing else in main defers anything that matters.
		if prof != nil {
			if err := prof.Stop(); err != nil {
				log.Printf("Failed to stop profiling: %v", err)
			}
		}
		os.Exit(code)
	}
}

// strictExitCode is what --strict exits with when a gate fails.
//
// Deliberately not 1: a CI script has to be able to tell "apispec could not
// run" from "apispec ran and the result is below the bar", because the second
// is a spec problem to look at and the first is a build problem. 2 is taken —
// the flag package exits with it on a usage error.
const strictExitCode = 3

// strictExit reports the exit code a --strict run should end with, after
// printing what failed.
func strictExit(config *CLIConfig, genEngine *engine.Engine) int {
	if !config.Strict.enabled {
		return 0
	}
	findings := genEngine.StrictFindings(config.Strict.categories)
	if len(findings) == 0 {
		return 0
	}
	// On stderr, alongside the warnings these mostly promote, and phrased so the
	// line stands on its own: in a folded CI log this may be the only thing read.
	//
	// Findings, not gates: one category can produce more than one — `schemas`
	// reports a response that lost its schema and a reference that lost its
	// component separately, because they are different things to go and look at.
	log.Printf("[strict] %d strict finding(s), the spec came out incomplete:", len(findings))
	for _, f := range findings {
		log.Printf("[strict]   %s", f)
	}
	return strictExitCode
}

// validateNaming rejects a naming style the spec layer does not know.
func validateNaming(flagName, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s: unknown style %q (want one of: %s)", flagName, value, strings.Join(allowed, ", "))
}
