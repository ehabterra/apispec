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
	"strings"
	"time"

	"github.com/ehabterra/swagen/internal/engine"
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
		// Get version from build info (usually the module version or VCS tag)
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}

		// Extract commit, build time, and Go version from build settings
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if len(setting.Value) >= 7 {
					Commit = setting.Value[:7] // Short commit hash
				} else {
					Commit = setting.Value
				}
			case "vcs.time":
				BuildDate = setting.Value
			}
		}

		// Get Go version from build info
		if info.GoVersion != "" {
			GoVersion = info.GoVersion
		}
	}

	// If we still don't have a meaningful version, try to detect from module info
	if Version == "0.0.1" || Version == "(devel)" {
		// This happens when installed via go install without a tagged version
		// We'll show a more informative message
		Version = "dev (installed via go install)"
	}
}

func printVersion() {
	// Detect version info if not already set via -ldflags
	detectVersionInfo()

	fmt.Printf("swagen version: %s\n", Version)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Printf("Build date: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
	fmt.Println(engine.CopyrightNotice)
	fmt.Println(engine.LicenseNotice)
}

// CLIConfig holds the configuration parsed from command line arguments
type CLIConfig struct {
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
	OutputConfig       string
	WriteMetadata      bool
	SplitMetadata      bool
	DiagramPath        string
	MaxNodesPerTree    int
	MaxChildrenPerNode int
	MaxArgsPerFunction int
	MaxNestedArgsDepth int
	ShowVersion        bool
	OutputFlagSet      bool
	IncludeFiles       []string
	IncludePackages    []string
	IncludeFunctions   []string
	IncludeTypes       []string
	ExcludeFiles       []string
	ExcludePackages    []string
	ExcludeFunctions   []string
	ExcludeTypes       []string
	SkipCGOPackages    bool
}

// parseFlags parses command line arguments and returns a CLIConfig
func parseFlags(args []string) (*CLIConfig, error) {
	// Create a new flag set to avoid global state
	fs := flag.NewFlagSet("swagen", flag.ContinueOnError)

	config := &CLIConfig{}

	// Version flag
	fs.BoolVar(&config.ShowVersion, "version", false, "Show version information")
	fs.BoolVar(&config.ShowVersion, "V", false, "Shorthand for --version")

	// Custom help
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n%s\n\nUsage: %s [flags]\n\nFlags:\n",
			engine.CopyrightNotice, engine.LicenseNotice, os.Args[0])
		fs.PrintDefaults()
		fmt.Printf("\nExamples:\n  %s -o spec.yaml -d ./api\n", os.Args[0])
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

	fs.IntVar(&config.MaxNodesPerTree, "max-nodes", engine.DefaultMaxNodesPerTree, "Maximum nodes per tracker tree")
	fs.IntVar(&config.MaxNodesPerTree, "mn", engine.DefaultMaxNodesPerTree, "Shorthand for --max-nodes")

	fs.IntVar(&config.MaxChildrenPerNode, "max-children", engine.DefaultMaxChildrenPerNode, "Maximum children per node")
	fs.IntVar(&config.MaxChildrenPerNode, "mc", engine.DefaultMaxChildrenPerNode, "Shorthand for --max-children")

	fs.IntVar(&config.MaxArgsPerFunction, "max-args", engine.DefaultMaxArgsPerFunction, "Maximum arguments per function")
	fs.IntVar(&config.MaxArgsPerFunction, "ma", engine.DefaultMaxArgsPerFunction, "Shorthand for --max-args")

	fs.IntVar(&config.MaxNestedArgsDepth, "max-nested-args", engine.DefaultMaxNestedArgsDepth, "Maximum nested arguments depth")
	fs.IntVar(&config.MaxNestedArgsDepth, "md", engine.DefaultMaxNestedArgsDepth, "Shorthand for --max-nested-args")

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

	if err := fs.Parse(args); err != nil {
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

	return config, nil
}

// runGeneration generates the OpenAPI specification based on the configuration
func runGeneration(config *CLIConfig) ([]byte, *engine.Engine, error) {
	// Create engine configuration
	engineConfig := &engine.EngineConfig{
		InputDir:           config.InputDir,
		OutputFile:         config.OutputFile,
		Title:              config.Title,
		APIVersion:         config.APIVersion,
		Description:        config.Description,
		TermsOfService:     config.TermsOfService,
		ContactName:        config.ContactName,
		ContactURL:         config.ContactURL,
		ContactEmail:       config.ContactEmail,
		LicenseName:        config.LicenseName,
		LicenseURL:         config.LicenseURL,
		OpenAPIVersion:     config.OpenAPIVersion,
		ConfigFile:         config.ConfigFile,
		OutputConfig:       config.OutputConfig,
		WriteMetadata:      config.WriteMetadata,
		SplitMetadata:      config.SplitMetadata,
		DiagramPath:        config.DiagramPath,
		MaxNodesPerTree:    config.MaxNodesPerTree,
		MaxChildrenPerNode: config.MaxChildrenPerNode,
		MaxArgsPerFunction: config.MaxArgsPerFunction,
		MaxNestedArgsDepth: config.MaxNestedArgsDepth,
		IncludeFiles:       config.IncludeFiles,
		IncludePackages:    config.IncludePackages,
		IncludeFunctions:   config.IncludeFunctions,
		IncludeTypes:       config.IncludeTypes,
		ExcludeFiles:       config.ExcludeFiles,
		ExcludePackages:    config.ExcludePackages,
		ExcludeFunctions:   config.ExcludeFunctions,
		ExcludeTypes:       config.ExcludeTypes,
		SkipCGOPackages:    config.SkipCGOPackages,
	}

	// Create engine and generate OpenAPI spec
	genEngine := engine.NewEngine(engineConfig)
	openAPISpec, err := genEngine.GenerateOpenAPI()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate OpenAPI spec: %w", err)
	}

	var data []byte
	ext := strings.ToLower(filepath.Ext(config.OutputFile))
	if ext == ".yaml" || ext == ".yml" {
		data, err = yaml.Marshal(openAPISpec)
	} else {
		data, err = json.MarshalIndent(openAPISpec, "", "  ")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal OpenAPI spec: %w", err)
	}

	return data, genEngine, nil
}

// writeOutput writes the generated data to the appropriate output destination
func writeOutput(data []byte, config *CLIConfig, genEngine *engine.Engine) error {
	// If output is the default (openapi.json) and no explicit output flag was set, output to stdout
	if config.OutputFile == engine.DefaultOutputFile && !config.OutputFlagSet {
		fmt.Print(string(data))
	} else {
		outputPath := filepath.Join(genEngine.ModuleRoot(), config.OutputFile)

		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Println("Successfully generated:", outputPath)
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

	// Generate OpenAPI specification
	data, genEngine, err := runGeneration(config)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Write output
	if err := writeOutput(data, config, genEngine); err != nil {
		log.Fatalf("%v", err)
	}

	fmt.Printf("Time elapsed: %s\n", time.Since(start))
}
