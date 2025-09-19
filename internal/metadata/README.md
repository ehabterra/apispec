# Metadata Package

This package provides functionality for generating metadata from Go source code, including function calls, assignments, and type information.

## Features

- **Function Analysis**: Extract function signatures, calls, and assignments
- **Type Information**: Analyze structs, interfaces, and generic types
- **Call Graph**: Build dependency graphs between functions
- **Package Analysis**: Process multiple packages and their relationships

## Usage

```go
import "github.com/ehabterra/apispec/internal/metadata"

// Generate metadata from Go packages
meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

// Write metadata to YAML file
err := metadata.WriteMetadata(meta, "output.yaml")
```

## Architecture

The package consists of several key components:

- **Generator**: Main entry point for metadata generation
- **Analyzer**: Core analysis logic for Go AST nodes
- **IO**: YAML serialization and file operations
- **Types**: Data structures for metadata representation

## Testing

Tests use the `packagestest` package to create temporary Go modules for testing various scenarios:

- Simple functions with variables and imports
- Struct types with methods and interfaces
- Generic functions and types
- Constants and variables
- Complex call graphs with method chains
- Multi-package dependencies

Each test validates the generated metadata against expected results without generating persistent files. 