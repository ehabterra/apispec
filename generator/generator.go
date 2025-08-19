// Package generator provides a simple, public API to generate OpenAPI specs
// from a Go project directory, matching the usage shown in README.
package generator

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"

	"github.com/ehabterra/swagen/internal/core"
	intmeta "github.com/ehabterra/swagen/internal/metadata"
	intspec "github.com/ehabterra/swagen/internal/spec"
	"github.com/ehabterra/swagen/spec"
	"golang.org/x/tools/go/packages"
)

const (
	defaultOpenAPIVersion     = "3.1.1"
	defaultMaxNodesPerTree    = 10000
	defaultMaxChildrenPerNode = 150
	defaultMaxArgsPerFunction = 30
	defaultMaxNestedArgsDepth = 50
)

// Generator encapsulates configuration and limits for generation.
type Generator struct {
	config         *spec.SwagenConfig
	openAPIVersion string
	limits         intspec.TrackerLimits
}

// NewGenerator creates a new Generator. If cfg is nil, a framework will be detected
// during generation and a default config will be used.
func NewGenerator(cfg *spec.SwagenConfig) *Generator {
	return &Generator{
		config:         cfg,
		openAPIVersion: defaultOpenAPIVersion,
		limits: intspec.TrackerLimits{
			MaxNodesPerTree:    defaultMaxNodesPerTree,
			MaxChildrenPerNode: defaultMaxChildrenPerNode,
			MaxArgsPerFunction: defaultMaxArgsPerFunction,
			MaxNestedArgsDepth: defaultMaxNestedArgsDepth,
		},
	}
}

// GenerateFromDirectory analyzes the Go module that contains dir and returns an OpenAPI spec.
func (g *Generator) GenerateFromDirectory(dir string) (*spec.OpenAPISpec, error) {
	if dir == "" {
		return nil, fmt.Errorf("directory path is required")
	}

	moduleRoot, err := findModuleRoot(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	fileToInfo := make(map[*ast.File]*types.Info)

	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:   moduleRoot,
		Fset:  fset,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("packages contain errors")
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

	meta := intmeta.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	// Determine configuration: use provided or detect framework defaults
	cfgToUse := g.config
	if cfgToUse == nil {
		detector := core.NewFrameworkDetector()
		framework, derr := detector.Detect(moduleRoot)
		if derr != nil {
			// Fallback to HTTP defaults if detection fails
			cfgToUse = spec.DefaultHTTPConfig()
		} else {
			switch framework {
			case "gin":
				cfgToUse = spec.DefaultGinConfig()
			case "chi":
				cfgToUse = spec.DefaultChiConfig()
			case "echo":
				cfgToUse = spec.DefaultEchoConfig()
			case "fiber":
				cfgToUse = spec.DefaultFiberConfig()
			default:
				cfgToUse = spec.DefaultHTTPConfig()
			}
		}
	}

	// Build tracker tree
	tree := intspec.NewTrackerTree(meta, g.limits)

	genCfg := intspec.GeneratorConfig{OpenAPIVersion: g.openAPIVersion}

	// Map to OpenAPI
	openAPISpec, err := intspec.MapMetadataToOpenAPI(tree, cfgToUse, genCfg)
	if err != nil {
		return nil, err
	}

	return openAPISpec, nil
}

// findModuleRoot ascends from startPath to locate go.mod and returns its directory.
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
			break
		}
		current = parent
	}
	return "", fmt.Errorf("no go.mod found in %s or any parent directory", startPath)
}
