// Package generator provides a simple, public API to generate OpenAPI specs
// from a Go project directory, matching the usage shown in README.
package generator

import (
	"fmt"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/spec"
)

// Generator encapsulates configuration and limits for generation.
type Generator struct {
	config *spec.APISpecConfig
	engine *engine.Engine
}

// NewGenerator creates a new Generator. If cfg is nil, a framework will be detected
// during generation and a default config will be used.
func NewGenerator(cfg *spec.APISpecConfig) *Generator {
	engineConfig := engine.DefaultEngineConfig()

	// If a config is provided, set it directly in the engine config
	if cfg != nil {
		engineConfig.APISpecConfig = cfg
	}

	return &Generator{
		config: cfg,
		engine: engine.NewEngine(engineConfig),
	}
}

// GenerateFromDirectory analyzes the Go module that contains dir and returns an OpenAPI spec.
func (g *Generator) GenerateFromDirectory(dir string) (*spec.OpenAPISpec, error) {
	if dir == "" {
		return nil, fmt.Errorf("directory path is required")
	}

	// Create a new engine config for this generation
	engineConfig := engine.DefaultEngineConfig()
	engineConfig.InputDir = dir

	// Pass the APISpecConfig directly to the engine
	if g.config != nil {
		engineConfig.APISpecConfig = g.config
	}

	// Create a new engine instance for this generation
	genEngine := engine.NewEngine(engineConfig)

	return genEngine.GenerateOpenAPI()
}
