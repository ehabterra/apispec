package core

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
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

	return "net/http", nil
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
