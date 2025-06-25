package cli

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
	iparser "github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
)

// FrameworkDetector detects the web framework used in a project
type FrameworkDetector struct{}

// NewFrameworkDetector creates a new framework detector
func NewFrameworkDetector() *FrameworkDetector {
	return &FrameworkDetector{}
}

// Detect determines which web framework is being used in the given directory
func (d *FrameworkDetector) Detect(dir string) (string, error) {
	// Collect Go files
	goFiles, err := CollectGoFiles(dir)
	if err != nil {
		return "", err
	}

	// Parse files to check for framework imports
	fset := token.NewFileSet()
	for _, filePath := range goFiles {
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			continue // Skip files that can't be parsed
		}

		// Check imports for framework indicators
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			switch {
			case strings.Contains(importPath, "gin-gonic/gin"):
				return "gin", nil
			case strings.Contains(importPath, "go-chi/chi"):
				return "chi", nil
			case strings.Contains(importPath, "labstack/echo"):
				return "echo", nil
			case strings.Contains(importPath, "gofiber/fiber"):
				return "fiber", nil
			}
		}
	}

	return "unknown", nil
}

// CollectGoFiles recursively collects all .go files from a directory
func CollectGoFiles(dir string) ([]string, error) {
	var goFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	return goFiles, err
}

// ParseGinProject parses a Gin project and returns parsed routes
func ParseGinProject(dir string) ([]core.ParsedRoute, error) {
	// This would be implemented to parse Gin projects
	// For now, return empty slice
	return []core.ParsedRoute{}, nil
}

// ParseChiProject parses a Chi project and returns parsed routes
func ParseChiProject(dir string) ([]core.ParsedRoute, error) {
	// This would be implemented to parse Chi projects
	// For now, return empty slice
	return []core.ParsedRoute{}, nil
}

// ParseEchoProject parses an Echo project and returns parsed routes
func ParseEchoProject(dir string) ([]core.ParsedRoute, error) {
	// This would be implemented to parse Echo projects
	// For now, return empty slice
	return []core.ParsedRoute{}, nil
}

// ParseFiberProject parses a Fiber project and returns parsed routes
func ParseFiberProject(dir string) ([]core.ParsedRoute, error) {
	// Collect Go files
	goFiles, err := CollectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var astFiles []*ast.File
	for _, filePath := range goFiles {
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err == nil {
			astFiles = append(astFiles, f)
		}
	}

	// Type checking
	conf := types.Config{Importer: importer.For("source", nil)}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	_, _ = conf.Check("main", fset, astFiles, info)

	p := iparser.DefaultFiberParserWithTypes(info)
	return p.Parse(fset, astFiles)
}

// WriteOpenAPISpec writes the OpenAPI specification to a file
func WriteOpenAPISpec(spec *spec.OpenAPISpec, outputFile, format string) error {
	// This would be implemented to write the spec to a file
	// For now, return nil
	return nil
}
