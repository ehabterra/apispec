# APISpec Generator Package

The `generator` package provides a simple, public API to generate OpenAPI specifications from Go project directories.

## Basic Usage

```go
package main

import (
    "fmt"
    "github.com/ehabterra/apispec/generator"
    "github.com/ehabterra/apispec/spec"
)

func main() {
    // Create a generator with default configuration
    gen := generator.NewGenerator(nil)
    
    // Generate OpenAPI spec from a directory
    spec, err := gen.GenerateFromDirectory("./my-api")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Generated API with %d paths\n", len(spec.Paths))
}
```

## Using Custom Configuration

You can pass a custom `APISpecConfig` object to customize the generation:

```go
package main

import (
    "fmt"
    "github.com/ehabterra/apispec/generator"
    "github.com/ehabterra/apispec/spec"
    intspec "github.com/ehabterra/apispec/internal/spec"
)

func main() {
    // Create a custom configuration
    customConfig := &spec.APISpecConfig{
        Info: spec.Info{
            Title:       "My Custom API",
            Description: "This is a custom API configuration",
            Version:     "2.0.0",
        },
        Framework: intspec.FrameworkConfig{
            RoutePatterns: []intspec.RoutePattern{
                {
                    CallRegex:      "^HandleFunc$",
                    PathFromArg:    true,
                    HandlerFromArg: true,
                    PathArgIndex:   0,
                    MethodArgIndex: -1,
                    HandlerArgIndex: 1,
                    RecvTypeRegex:  "^net/http(\\.\\*ServeMux)?$",
                },
            },
        },
        Defaults: intspec.Defaults{
            RequestContentType:  "application/json",
            ResponseContentType: "application/json",
            ResponseStatus:      200,
        },
    }
    
    // Create generator with custom config
    gen := generator.NewGenerator(customConfig)
    
    // Generate OpenAPI spec
    spec, err := gen.GenerateFromDirectory("./my-api")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Generated API: %s v%s\n", spec.Info.Title, spec.Info.Version)
}
```

## Configuration Priority

The generator follows this priority order for configuration:

1. **Direct APISpecConfig object** (highest priority) - If passed to `NewGenerator()`
2. **Config file** - If specified via `ConfigFile` field
3. **Auto-detected framework defaults** - If no config is provided

## Features

- **Framework Detection**: Automatically detects Gin, Chi, Echo, Fiber, or net/http
- **Custom Patterns**: Define custom route extraction patterns
- **Type Mapping**: Map Go types to OpenAPI schemas
- **External Types**: Handle external types like database primitives
- **Metadata Generation**: Generate detailed metadata about your codebase

## Error Handling

The generator returns descriptive errors for common issues:

- Invalid directories
- Missing Go modules
- Syntax errors in Go code
- Configuration file issues

## Integration

This package is designed to work seamlessly with the main `apispec` CLI tool and can be used as a library in your own Go applications.
