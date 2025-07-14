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
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/metadata"
	"github.com/ehabterra/swagen/internal/spec"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// findModuleRoot finds the root directory of a Go module by looking for go.mod
func findModuleRoot(startPath string) (string, error) {
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

// main is the entry point for the CLI tool. It parses flags, collects Go files, runs type-checking, detects the framework, parses routes, and generates the OpenAPI spec.
func main() {
	start := time.Now()
	// Print copyright and license info at the very start
	fmt.Println("swagen - Copyright 2025 Ehab Terra")
	fmt.Println("Licensed under the Apache License 2.0. See LICENSE and NOTICE.")

	// --- CLI Flags ---
	output := flag.String("o", "openapi.json", "Output file for the OpenAPI specification (e.g., openapi.json, openapi.yaml)")
	inputDir := flag.String("d", ".", "Directory to parse for Go source files")
	// excludeDirs := flag.String("exclude", "vendor,testdata,mocks", "A comma-separated list of directories to exclude from parsing.")
	// Metadata flags
	title := flag.String("title", "Generated API", "API Title")
	apiVersion := flag.String("api.version", "1.0.0", "API Version")
	description := flag.String("description", "", "API Description")
	termsOfService := flag.String("terms", "", "Terms of Service URL")
	contactName := flag.String("contact.name", "Ehab", "Contact Name")
	contactURL := flag.String("contact.url", "https://ehabterra.github.io/", "Contact URL")
	contactEmail := flag.String("contact.email", "ehabterra@hotmail.com", "Contact Email")
	licenseName := flag.String("license.name", "", "License Name")
	licenseURL := flag.String("license.url", "", "License URL")
	openapiVersion := flag.String("openapi.version", "3.1.1", "OpenAPI Specification version (e.g., 3.1.1, 3.0.3)")
	// Metadata output flags
	splitMetadata := flag.Bool("split-metadata", false, "Split metadata into separate files (string-pool, packages, call-graph)")
	configFile := flag.String("config", "", "Path to custom Swagen config YAML file")
	maxNodesPerTree := flag.Int("max-nodes-per-tree", 10000, "Maximum number of nodes allowed in a single call graph tree (prevents infinite loops)")
	maxChildrenPerNode := flag.Int("max-children-per-node", 150, "Maximum number of children allowed per node in the call graph tree")
	maxArgsPerFunction := flag.Int("max-args-per-function", 20, "Maximum number of arguments to process per function call in the call graph tree")
	maxNestedArgsDepth := flag.Int("max-nested-args-depth", 100, "Maximum depth for collecting nested argument IDs in the call graph tree")
	outputConfig := flag.String("output-config", "", "Output the effective/used config (after CLI overrides) to this YAML file")
	writeMetadata := flag.Bool("write-metadata", false, "Write metadata.yaml or split metadata files to disk")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "swagen - Copyright 2025 Ehab Terra\n")
		fmt.Fprintf(os.Stderr, "Licensed under the Apache License 2.0. See LICENSE and NOTICE.\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	targetPath, err := filepath.Abs(*inputDir) // your target path
	if err != nil {
		log.Fatalf("Could not find Go module: %v", err)
	}

	// Find and switch to module root
	moduleRoot, err := findModuleRoot(targetPath)
	if err != nil {
		log.Fatalf("Could not find Go module: %v", err)
	}

	// Change working directory to module root
	originalWd, _ := os.Getwd()
	os.Chdir(moduleRoot)
	defer os.Chdir(originalWd)

	fset := token.NewFileSet()
	fileToInfo := make(map[*ast.File]*types.Info)
	var allFiles []*ast.File

	log.Println("Starting to load and type-check packages...")
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:   moduleRoot,
		Fset:  fset,
		Tests: false, // Explicitly exclude test files to speed up processing
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatalf("failed to load packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		log.Fatalf("packages contain errors")
	}

	// --- Generate and save metadata ---
	// Group files by package for metadata
	pkgsMetadata := make(map[string]map[string]*ast.File)
	importPaths := make(map[string]string)

	for _, pkg := range pkgs {
		pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)

		for i, f := range pkg.Syntax {
			pkgsMetadata[pkg.PkgPath][pkg.GoFiles[i]] = f
			allFiles = append(allFiles, f)
			fileToInfo[f] = pkg.TypesInfo
			importPaths[pkg.GoFiles[i]] = pkg.PkgPath // fallback, can be improved
		}
	}

	// // Build funcMap for AST-based handler analysis
	// funcMap := parser.BuildFuncMap(allFiles)

	log.Println("Finished loading and type-checking packages.")

	// --- Detect framework (Gin, Echo, etc.) ---
	detector := core.NewFrameworkDetector()
	framework, err := detector.Detect(moduleRoot)
	if err != nil {
		panic(err)
	}
	fmt.Println("Detected framework:", framework)

	// --- Load SwagenConfig: custom or default per framework ---
	var swagenConfig *spec.SwagenConfig
	if *configFile != "" {
		swagenConfig, err = spec.LoadSwagenConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
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

	// After parsing flags:
	desc := *description
	licenseNotice := "\n\nCopyright 2025 Ehab Terra. Licensed under the Apache License 2.0. See LICENSE and NOTICE."
	if !strings.HasSuffix(desc, licenseNotice) {
		desc += licenseNotice
	}
	info := spec.Info{
		Title:          *title, // assuming you have a title flag
		Description:    desc,
		Version:        *apiVersion,
		TermsOfService: *termsOfService,
		Contact: &spec.Contact{
			Name:  *contactName,
			URL:   *contactURL,
			Email: *contactEmail,
		},
		License: &spec.License{
			Name: *licenseName,
			URL:  *licenseURL,
		},
	}

	// Set this info on your config (assuming swagenConfig is your config variable)
	swagenConfig.Info = info

	// If --output-config is set, write the effective config to the specified file
	if *outputConfig != "" {
		cfgYaml, err := yaml.Marshal(swagenConfig)
		if err != nil {
			log.Fatalf("Failed to marshal effective config: %v", err)
		}
		err = os.WriteFile(*outputConfig, cfgYaml, 0644)
		if err != nil {
			log.Fatalf("Failed to write effective config to %s: %v", *outputConfig, err)
		}
		fmt.Printf("Effective config written to %s\n", *outputConfig)
	}

	// Updated: Only two return values, no disableStringPool
	meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	// Write metadata (split or combined) only if --write-metadata is set
	if *writeMetadata {
		if *splitMetadata {
			if err := metadata.WriteSplitMetadata(meta, "metadata.yaml"); err != nil {
				log.Printf("Failed to write split metadata files: %v", err)
			} else {
				log.Println("Successfully wrote split metadata files:")
				log.Println("  - metadata-string-pool.yaml")
				log.Println("  - metadata-packages.yaml")
				log.Println("  - metadata-call-graph.yaml")
			}
		} else {
			if err := metadata.WriteMetadata(meta, "metadata.yaml"); err != nil {
				log.Printf("Failed to write metadata.yaml: %v", err)
			} else {
				log.Println("Successfully wrote metadata.yaml file")
			}
		}
	}

	// --- Prepare OpenAPI generator config ---
	config := spec.GeneratorConfig{
		OpenAPIVersion: *openapiVersion,
		Title:          *title,
		APIVersion:     *apiVersion,
	}

	// Construct the tree
	limits := spec.TrackerLimits{
		MaxNodesPerTree:    *maxNodesPerTree,
		MaxChildrenPerNode: *maxChildrenPerNode,
		MaxArgsPerFunction: *maxArgsPerFunction,
		MaxNestedArgsDepth: *maxNestedArgsDepth,
	}
	tree := spec.NewTrackerTree(meta, limits)

	spec.GenerateCytoscapeHTML(tree.GetRoots(), "./diagram.html")

	// --- Generate OpenAPI spec from metadata using config-driven extractor/mapper ---
	openAPISpec, err := spec.MapMetadataToOpenAPI(tree, swagenConfig, config)
	if err != nil {
		panic(err)
	}

	// --- Output OpenAPI spec as JSON or YAML based on file extension ---
	var data []byte
	ext := strings.ToLower(filepath.Ext(*output))
	if ext == ".yaml" || ext == ".yml" {
		data, err = yaml.Marshal(openAPISpec)
	} else {
		data, err = json.MarshalIndent(openAPISpec, "", "  ")
	}

	if err != nil {
		panic(fmt.Errorf("failed to marshal spec: %w", err))
	}

	if err := os.WriteFile(*output, data, 0644); err != nil {
		panic(fmt.Errorf("failed to write output file: %w", err))
	}

	fmt.Println("Successfully generated:", *output)
	fmt.Printf("Time elapsed: %s\n", time.Since(start))
}
