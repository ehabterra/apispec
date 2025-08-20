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
	"strings"
	"time"

	"github.com/ehabterra/swagen/internal/engine"
	"gopkg.in/yaml.v3"
)

const (
	// Version info injected at build time via -ldflags
	Version   = "0.0.1"
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

func printVersion() {
	fmt.Printf("swagen version: %s\n", Version)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Printf("Build date: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
	fmt.Println(engine.CopyrightNotice)
	fmt.Println(engine.LicenseNotice)
}

func main() {
	start := time.Now()
	// Print copyright and license info at the very start
	fmt.Println(engine.CopyrightNotice)

	// Version flag
	showVersion := flag.Bool("version", false, "Show version information")
	flag.BoolVar(showVersion, "V", false, "Shorthand for --version")

	// Custom help
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n%s\n\nUsage: %s [flags]\n\nFlags:\n",
			engine.CopyrightNotice, engine.LicenseNotice, os.Args[0])
		flag.PrintDefaults()
		fmt.Printf("\nExamples:\n  %s -o spec.yaml -d ./api\n", os.Args[0])
	}

	// Parse flags
	inputDir := flag.String("dir", engine.DefaultInputDir, "Input directory containing Go source files")
	flag.StringVar(inputDir, "d", engine.DefaultInputDir, "Shorthand for --dir")

	outputFile := flag.String("output", engine.DefaultOutputFile, "Output file path")
	flag.StringVar(outputFile, "o", engine.DefaultOutputFile, "Shorthand for --output")

	title := flag.String("title", engine.DefaultTitle, "API title")
	flag.StringVar(title, "t", engine.DefaultTitle, "Shorthand for --title")

	apiVersion := flag.String("api-version", engine.DefaultAPIVersion, "API version")
	flag.StringVar(apiVersion, "v", engine.DefaultAPIVersion, "Shorthand for --api-version")

	description := flag.String("description", "", "API description")
	flag.StringVar(description, "D", "", "Shorthand for --description")

	termsOfService := flag.String("terms", "", "Terms of service URL")
	flag.StringVar(termsOfService, "T", "", "Shorthand for --terms")

	contactName := flag.String("contact-name", engine.DefaultContactName, "Contact name")
	flag.StringVar(contactName, "N", engine.DefaultContactName, "Shorthand for --contact-name")

	contactURL := flag.String("contact-url", engine.DefaultContactURL, "Contact URL")
	flag.StringVar(contactURL, "U", engine.DefaultContactURL, "Shorthand for --contact-url")

	contactEmail := flag.String("contact-email", engine.DefaultContactEmail, "Contact email")
	flag.StringVar(contactEmail, "E", engine.DefaultContactEmail, "Shorthand for --contact-email")

	licenseName := flag.String("license-name", "", "License name")
	flag.StringVar(licenseName, "L", "", "Shorthand for --license-name")

	licenseURL := flag.String("license-url", "", "License URL")
	flag.StringVar(licenseURL, "lu", "", "Shorthand for --license-url")

	openAPIVersion := flag.String("openapi-version", engine.DefaultOpenAPIVersion, "OpenAPI specification version")
	flag.StringVar(openAPIVersion, "O", engine.DefaultOpenAPIVersion, "Shorthand for --openapi-version")

	configFile := flag.String("config", "", "Configuration file path")
	flag.StringVar(configFile, "c", "", "Shorthand for --config")

	outputConfig := flag.String("output-config", "", "Output effective configuration to file")
	flag.StringVar(outputConfig, "oc", "", "Shorthand for --output-config")

	writeMetadata := flag.Bool("write-metadata", false, "Write metadata to file")
	flag.BoolVar(writeMetadata, "w", false, "Shorthand for --write-metadata")

	splitMetadata := flag.Bool("split-metadata", false, "Write split metadata files")
	flag.BoolVar(splitMetadata, "s", false, "Shorthand for --split-metadata")

	diagramPath := flag.String("diagram", "", "Generate call graph diagram")
	flag.StringVar(diagramPath, "g", "", "Shorthand for --diagram")

	maxNodesPerTree := flag.Int("max-nodes", engine.DefaultMaxNodesPerTree, "Maximum nodes per tracker tree")
	flag.IntVar(maxNodesPerTree, "mn", engine.DefaultMaxNodesPerTree, "Shorthand for --max-nodes")

	maxChildrenPerNode := flag.Int("max-children", engine.DefaultMaxChildrenPerNode, "Maximum children per node")
	flag.IntVar(maxChildrenPerNode, "mc", engine.DefaultMaxChildrenPerNode, "Shorthand for --max-children")

	maxArgsPerFunction := flag.Int("max-args", engine.DefaultMaxArgsPerFunction, "Maximum arguments per function")
	flag.IntVar(maxArgsPerFunction, "ma", engine.DefaultMaxArgsPerFunction, "Shorthand for --max-args")

	maxNestedArgsDepth := flag.Int("max-nested-args", engine.DefaultMaxNestedArgsDepth, "Maximum nested arguments depth")
	flag.IntVar(maxNestedArgsDepth, "md", engine.DefaultMaxNestedArgsDepth, "Shorthand for --max-nested-args")

	flag.Parse()

	// Handle version flag early
	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	// Handle positional arguments (override --dir flag)
	if len(flag.Args()) > 0 {
		*inputDir = flag.Args()[0]
	}

	// Create engine configuration
	engineConfig := &engine.EngineConfig{
		InputDir:           *inputDir,
		OutputFile:         *outputFile,
		Title:              *title,
		APIVersion:         *apiVersion,
		Description:        *description,
		TermsOfService:     *termsOfService,
		ContactName:        *contactName,
		ContactURL:         *contactURL,
		ContactEmail:       *contactEmail,
		LicenseName:        *licenseName,
		LicenseURL:         *licenseURL,
		OpenAPIVersion:     *openAPIVersion,
		ConfigFile:         *configFile,
		OutputConfig:       *outputConfig,
		WriteMetadata:      *writeMetadata,
		SplitMetadata:      *splitMetadata,
		DiagramPath:        *diagramPath,
		MaxNodesPerTree:    *maxNodesPerTree,
		MaxChildrenPerNode: *maxChildrenPerNode,
		MaxArgsPerFunction: *maxArgsPerFunction,
		MaxNestedArgsDepth: *maxNestedArgsDepth,
	}

	// Create engine and generate OpenAPI spec
	genEngine := engine.NewEngine(engineConfig)
	openAPISpec, err := genEngine.GenerateOpenAPI()
	if err != nil {
		log.Fatalf("Failed to generate OpenAPI spec: %v", err)
	}

	var data []byte
	ext := strings.ToLower(filepath.Ext(*outputFile))
	if ext == ".yaml" || ext == ".yml" {
		data, err = yaml.Marshal(openAPISpec)
	} else {
		data, err = json.MarshalIndent(openAPISpec, "", "  ")
	}
	if err != nil {
		log.Fatalf("Failed to marshal OpenAPI spec: %v", err)
	}

	// Check if output flag was explicitly set
	outputFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "output" {
			outputFlagSet = true
		}
	})

	// If output is the default (openapi.json) and no explicit output flag was set, output to stdout
	if *outputFile == engine.DefaultOutputFile && !outputFlagSet {
		fmt.Print(string(data))
	} else {
		outputPath := filepath.Join(genEngine.ModuleRoot(), *outputFile)

		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			panic(fmt.Errorf("failed to write output file: %w", err))
		}
		fmt.Println("Successfully generated:", outputPath)
	}
	fmt.Printf("Time elapsed: %s\n", time.Since(start))
}
