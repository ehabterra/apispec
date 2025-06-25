package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	goparser "go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ehabterra/swagen/internal/cli"
	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
	"gopkg.in/yaml.v3"
)

// main is the entry point for the CLI tool. It parses flags, collects Go files, runs type-checking, detects the framework, parses routes, and generates the OpenAPI spec.
func main() {
	// --- CLI Flags ---
	output := flag.String("o", "openapi.json", "Output file for the OpenAPI specification (e.g., openapi.json, openapi.yaml)")
	inputDir := flag.String("d", ".", "Directory to parse for Go source files")
	// Metadata flags
	title := flag.String("title", "Generated API", "API Title")
	description := flag.String("description", "", "API Description")
	apiVersion := flag.String("api.version", "1.0.0", "API Version")
	termsOfService := flag.String("terms", "", "Terms of Service URL")
	contactName := flag.String("contact.name", "", "Contact Name")
	contactURL := flag.String("contact.url", "", "Contact URL")
	contactEmail := flag.String("contact.email", "", "Contact Email")
	licenseName := flag.String("license.name", "", "License Name")
	licenseURL := flag.String("license.url", "", "License URL")
	openapiVersion := flag.String("openapi.version", "3.1.1", "OpenAPI Specification version (e.g., 3.1.1, 3.0.3)")
	flag.Parse()

	// --- Recursively collect all .go files ---
	var goFiles []string
	err := filepath.WalkDir(*inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	// --- Parse all files into *ast.File objects ---
	fset := token.NewFileSet()
	var files []*ast.File
	for _, filePath := range goFiles {
		f, err := goparser.ParseFile(fset, filePath, nil, goparser.ParseComments)
		if err != nil {
			panic(err)
		}
		files = append(files, f)
	}

	// --- Set up go/types type-checking for accurate type resolution ---
	conf := types.Config{Importer: importer.For("source", nil)}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	_, err = conf.Check("main", fset, files, info)
	if err != nil {
		panic(err)
	}

	// --- Detect framework (Gin, Echo, etc.) ---
	detector := cli.NewFrameworkDetector()
	framework, err := detector.Detect(*inputDir)
	if err != nil {
		panic(err)
	}
	fmt.Println("Detected framework:", framework)

	// --- Select parser based on detected framework ---
	var routes []core.ParsedRoute
	switch framework {
	case "gin":
		p := parser.DefaultGinParserWithTypes(info)
		routes, err = p.Parse(fset, files)
	case "chi":
		p := parser.DefaultChiParserWithTypes(info)
		routes, err = p.Parse(fset, files)
	case "echo":
		p := parser.DefaultEchoParserWithTypes(info)
		routes, err = p.Parse(fset, files)
	case "fiber":
		p := parser.DefaultFiberParserWithTypes(info)
		routes, err = p.Parse(fset, files)
	default:
		fmt.Println("No parser implemented for framework:", framework)
		os.Exit(1)
	}

	if err != nil {
		panic(err)
	}

	// --- Generate OpenAPI spec from parsed routes ---
	config := spec.GeneratorConfig{
		OpenAPIVersion: *openapiVersion,
		Title:          *title,
		Description:    *description,
		APIVersion:     *apiVersion,
		TermsOfService: *termsOfService,
		ContactName:    *contactName,
		ContactURL:     *contactURL,
		ContactEmail:   *contactEmail,
		LicenseName:    *licenseName,
		LicenseURL:     *licenseURL,
	}
	gen := spec.NewOpenAPIGenerator(config)
	openAPISpec, err := gen.GenerateFromRoutes(routes, files)
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
}
