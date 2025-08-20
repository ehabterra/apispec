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

const (
	defaultOutputFile         = "openapi.json"
	defaultInputDir           = "."
	defaultTitle              = "Generated API"
	defaultAPIVersion         = "1.0.0"
	defaultContactName        = "Ehab"
	defaultContactURL         = "https://ehabterra.github.io/"
	defaultContactEmail       = "ehabterra@hotmail.com"
	defaultOpenAPIVersion     = "3.1.1"
	defaultMaxNodesPerTree    = 10000
	defaultMaxChildrenPerNode = 150
	defaultMaxArgsPerFunction = 30
	defaultMaxNestedArgsDepth = 50
	defaultMetadataFile       = "metadata.yaml"
	copyrightNotice           = "swagen - Copyright 2025 Ehab Terra"
	licenseNotice             = "Licensed under the Apache License 2.0. See LICENSE and NOTICE."
	fullLicenseNotice         = "\n\nCopyright 2025 Ehab Terra. Licensed under the Apache License 2.0. See LICENSE and NOTICE."
)

// Version info injected at build time via -ldflags
var (
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
	fmt.Println(copyrightNotice)
	fmt.Println(licenseNotice)
}

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

func main() {
	start := time.Now()
	// Print copyright and license info at the very start
	fmt.Println(copyrightNotice)
	fmt.Println(licenseNotice)

	// --- CLI Flags ---
	output := flag.String("output", defaultOutputFile, "Output file for OpenAPI spec (e.g., openapi.json)")
	flag.StringVar(output, "o", defaultOutputFile, "Shorthand for --output")

	// Track if output flag was explicitly set
	var outputFlagSet bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "output" || f.Name == "o" {
			outputFlagSet = true
		}
	})

	inputDir := flag.String("dir", defaultInputDir, "Directory to parse for Go files")
	flag.StringVar(inputDir, "d", defaultInputDir, "Shorthand for --dir")

	title := flag.String("title", defaultTitle, "API title")
	flag.StringVar(title, "t", defaultTitle, "Shorthand for --title")

	apiVersion := flag.String("api-version", defaultAPIVersion, "API version")
	flag.StringVar(apiVersion, "v", defaultAPIVersion, "Shorthand for --api-version")

	description := flag.String("description", "", "API description")
	flag.StringVar(description, "D", "", "Shorthand for --description")

	termsOfService := flag.String("terms-url", "", "Terms of Service URL")
	flag.StringVar(termsOfService, "T", "", "Shorthand for --terms-url")

	contactName := flag.String("contact-name", defaultContactName, "Contact name")
	flag.StringVar(contactName, "N", defaultContactName, "Shorthand for --contact-name")

	contactURL := flag.String("contact-url", defaultContactURL, "Contact URL")
	flag.StringVar(contactURL, "U", defaultContactURL, "Shorthand for --contact-url")

	contactEmail := flag.String("contact-email", defaultContactEmail, "Contact email")
	flag.StringVar(contactEmail, "E", defaultContactEmail, "Shorthand for --contact-email")

	licenseName := flag.String("license-name", "", "License name")
	flag.StringVar(licenseName, "L", "", "Shorthand for --license-name")

	licenseURL := flag.String("license-url", "", "License URL")
	flag.StringVar(licenseURL, "lu", "", "Shorthand for --license-url")

	openapiVersion := flag.String("openapi-version", defaultOpenAPIVersion, "OpenAPI spec version")
	flag.StringVar(openapiVersion, "O", defaultOpenAPIVersion, "Shorthand for --openapi-version")

	configFile := flag.String("config", "", "Path to custom config YAML")
	flag.StringVar(configFile, "c", "", "Shorthand for --config")

	outputConfig := flag.String("output-config", "", "Output effective config to YAML")
	flag.StringVar(outputConfig, "oc", "", "Shorthand for --output-config")

	writeMetadata := flag.Bool("write-metadata", false, "Write metadata.yaml to disk")
	flag.BoolVar(writeMetadata, "w", false, "Shorthand for --write-metadata")

	splitMetadata := flag.Bool("split-metadata", false, "Split metadata into separate files")
	flag.BoolVar(splitMetadata, "s", false, "Shorthand for --split-metadata")

	diagramPath := flag.String("diagram", "", "Save call graph as HTML")
	flag.StringVar(diagramPath, "g", "", "Shorthand for --diagram")

	maxNodesPerTree := flag.Int("max-nodes", defaultMaxNodesPerTree, "Max nodes in call graph tree")
	flag.IntVar(maxNodesPerTree, "mn", defaultMaxNodesPerTree, "Shorthand for --max-nodes")

	maxChildrenPerNode := flag.Int("max-children", defaultMaxChildrenPerNode, "Max children per node")
	flag.IntVar(maxChildrenPerNode, "mc", defaultMaxChildrenPerNode, "Shorthand for --max-children")

	maxArgsPerFunction := flag.Int("max-args", defaultMaxArgsPerFunction, "Max arguments per function")
	flag.IntVar(maxArgsPerFunction, "ma", defaultMaxArgsPerFunction, "Shorthand for --max-args")

	maxNestedArgsDepth := flag.Int("max-depth", defaultMaxNestedArgsDepth, "Max depth for nested args")
	flag.IntVar(maxNestedArgsDepth, "md", defaultMaxNestedArgsDepth, "Shorthand for --max-depth")

	showVersion := flag.Bool("version", false, "Show version information")
	flag.BoolVar(showVersion, "V", false, "Shorthand for --version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n%s\n\nUsage: %s [flags]\n\nFlags:\n",
			copyrightNotice, licenseNotice, os.Args[0])
		flag.PrintDefaults()
		fmt.Printf("\nExamples:\n  %s -o spec.yaml -d ./api\n", os.Args[0])
	}

	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	if len(flag.Args()) > 0 {
		*inputDir = flag.Args()[0]
	}

	targetPath, err := filepath.Abs(*inputDir)
	if err != nil {
		log.Fatalf("Could not find Go module: %v", err)
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		log.Fatalf("Input directory does not exist: %s", targetPath)
	}

	moduleRoot, err := findModuleRoot(targetPath)
	if err != nil {
		log.Fatalf("Could not find Go module: %v", err)
	}

	originalWd, _ := os.Getwd()
	os.Chdir(moduleRoot)
	defer os.Chdir(originalWd)

	fset := token.NewFileSet()
	fileToInfo := make(map[*ast.File]*types.Info)

	log.Println("Starting to load and type-check packages...")
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:   moduleRoot,
		Fset:  fset,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatalf("failed to load packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		log.Fatalf("packages contain errors")
	}

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

	log.Println("Finished loading and type-checking packages.")

	detector := core.NewFrameworkDetector()
	framework, err := detector.Detect(moduleRoot)
	if err != nil {
		panic(err)
	}
	fmt.Println("Detected framework:", framework)

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
			swagenConfig = spec.DefaultHTTPConfig()
		}
	}

	desc := *description
	licenseNotice := fullLicenseNotice
	if !strings.HasSuffix(desc, licenseNotice) {
		desc += licenseNotice
	}
	info := spec.Info{
		Title:          *title,
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

	swagenConfig.Info = info

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

	meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	if *writeMetadata {
		if *splitMetadata {
			if err := metadata.WriteSplitMetadata(meta, defaultMetadataFile); err != nil {
				log.Printf("Failed to write split metadata files: %v", err)
			} else {
				log.Println("Successfully wrote split metadata files:")
				log.Println("  - metadata-string-pool.yaml")
				log.Println("  - metadata-packages.yaml")
				log.Println("  - metadata-call-graph.yaml")
			}
		} else {
			if err := metadata.WriteMetadata(meta, defaultMetadataFile); err != nil {
				log.Printf("Failed to write metadata.yaml: %v", err)
			} else {
				log.Println("Successfully wrote metadata.yaml file")
			}
		}
	}

	config := spec.GeneratorConfig{
		OpenAPIVersion: *openapiVersion,
		Title:          *title,
		APIVersion:     *apiVersion,
	}

	limits := spec.TrackerLimits{
		MaxNodesPerTree:    *maxNodesPerTree,
		MaxChildrenPerNode: *maxChildrenPerNode,
		MaxArgsPerFunction: *maxArgsPerFunction,
		MaxNestedArgsDepth: *maxNestedArgsDepth,
	}
	tree := spec.NewTrackerTree(meta, limits)

	if *diagramPath != "" {
		err := spec.GenerateCytoscapeHTML(tree.GetRoots(), *diagramPath)
		if err != nil {
			log.Printf("Failed to generate diagram HTML: %v", err)
		} else {
			log.Printf("Diagram HTML written to %s", *diagramPath)
		}
	}

	openAPISpec, err := spec.MapMetadataToOpenAPI(tree, swagenConfig, config)
	if err != nil {
		panic(err)
	}

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

	if *output == defaultOutputFile && !outputFlagSet {
		fmt.Print(string(data))
	} else {
		if err := os.WriteFile(*output, data, 0644); err != nil {
			panic(fmt.Errorf("failed to write output file: %w", err))
		}
		fmt.Println("Successfully generated:", *output)
	}
	fmt.Printf("Time elapsed: %s\n", time.Since(start))
}
