package swagen

import (
	"go/ast"

	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/spec"
)

// Generator provides a high-level interface for generating OpenAPI specifications
type Generator struct {
	config spec.GeneratorConfig
}

// NewGenerator creates a new OpenAPI generator with the given configuration
func NewGenerator(config spec.GeneratorConfig) *Generator {
	return &Generator{
		config: config,
	}
}

// GenerateFromDirectory generates an OpenAPI specification from a directory containing Go files
func (g *Generator) GenerateFromDirectory(dir string) (*spec.OpenAPISpec, error) {
	// Detect framework
	detector := core.NewFrameworkDetector()
	framework, err := detector.Detect(dir)
	if err != nil {
		return nil, err
	}

	// Parse routes based on framework
	var routes []core.ParsedRoute
	switch framework {
	case "gin":
		routes, err = g.parseGinProject(dir)
	case "chi":
		routes, err = g.parseChiProject(dir)
	case "echo":
		routes, err = g.parseEchoProject(dir)
	default:
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	// Collect Go files for type resolution
	goFiles, err := g.collectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	return spec.MapParsedRoutesToOpenAPI(routes, goFiles, g.config)
}

// GenerateFromRoutes generates an OpenAPI specification from parsed routes
func (g *Generator) GenerateFromRoutes(routes []core.ParsedRoute, goFiles []*ast.File) (*spec.OpenAPISpec, error) {
	return spec.MapParsedRoutesToOpenAPI(routes, goFiles, g.config)
}

// Helper methods for parsing different frameworks
func (g *Generator) parseGinProject(dir string) ([]core.ParsedRoute, error) {
	// Implementation would go here
	return []core.ParsedRoute{}, nil
}

func (g *Generator) parseChiProject(dir string) ([]core.ParsedRoute, error) {
	// Implementation would go here
	return []core.ParsedRoute{}, nil
}

func (g *Generator) parseEchoProject(dir string) ([]core.ParsedRoute, error) {
	// Implementation would go here
	return []core.ParsedRoute{}, nil
}

func (g *Generator) collectGoFiles(dir string) ([]*ast.File, error) {
	// Implementation would go here
	return []*ast.File{}, nil
}

// Convenience functions for common use cases
func GenerateFromGin(dir string, config spec.GeneratorConfig) (*spec.OpenAPISpec, error) {
	gen := NewGenerator(config)
	return gen.GenerateFromDirectory(dir)
}

func GenerateFromChi(dir string, config spec.GeneratorConfig) (*spec.OpenAPISpec, error) {
	gen := NewGenerator(config)
	return gen.GenerateFromDirectory(dir)
}

func GenerateFromEcho(dir string, config spec.GeneratorConfig) (*spec.OpenAPISpec, error) {
	gen := NewGenerator(config)
	return gen.GenerateFromDirectory(dir)
}
