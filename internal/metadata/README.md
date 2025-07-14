# Metadata Package

This package provides functionality for extracting and managing metadata from Go source code, including call graphs, type information, and function relationships.

## Structure

The package is organized into several files for better maintainability and separation of concerns:

### `types.go`
Contains all data structures and type definitions:
- `StringPool` - For deduplicating strings across metadata
- `Metadata` - Main metadata structure with compressed string references
- All related types for packages, files, types, functions, etc.

### `helpers.go`
Contains utility functions for AST processing:
- `getTypeName()` - Extracts type names from AST expressions
- `getPosition()` - Converts token positions to strings
- `getComments()` - Extracts comments from AST nodes
- `getScope()` - Determines if a name is exported/unexported
- `formatSignature()` - Creates readable function signatures

### `expression.go`
Handles expression parsing and conversion:
- `ExprToCallArgument()` - Converts AST expressions to structured call arguments
- `getTypeString()` - Extracts type strings from AST expressions
- `callArgToString()` - Converts call arguments to string representations
- Various handler functions for different expression types

### `analysis.go`
Contains analysis logic for call graphs and type relationships:
- `scopeOf()` - Determines if a name is exported/unexported
- `implementsInterface()` - Checks if a struct implements an interface
- `getEnclosingFunctionName()` - Finds the function containing a position
- `getCalleeFunctionNameAndPackage()` - Extracts function call information
- Simplified assignment tracking and concrete type analysis

### `io.go`
Handles all I/O operations:
- `WriteMetadata()` - Writes metadata to YAML
- `LoadMetadata()` - Loads metadata from YAML files
- `WriteSplitMetadata()` - Writes metadata split into multiple files
- `LoadSplitMetadata()` - Loads metadata from split files

### `metadata.go`
Main entry point and core logic:
- `FillMetadataAndCallGraph()` - Main function for extracting metadata
- Contains the main processing logic for packages, files, types, and call graphs
- Modular processing functions for different AST elements

## Key Features

### String Pool Compression
The package uses a string pool to deduplicate strings across metadata, significantly reducing file sizes while maintaining functionality.

### Call Graph Analysis
Tracks function calls and their relationships, including:
- Caller and callee information
- Function arguments
- Method calls on concrete types
- Interface implementation detection

### Type System Analysis
- Struct and interface definitions
- Method signatures and implementations
- Field information and tags
- Type relationships and embeddings

### Simplified Assignment Tracking
Tracks variable assignments to determine concrete types that implement interfaces, enabling better call graph analysis.

## Usage

```go
// Extract metadata from Go packages
metadata, callGraph := FillMetadataAndCallGraph(
    pkgs,           // map[string]map[string]*ast.File
    fileToInfo,     // map[*ast.File]*types.Info
    importPaths,    // map[string]string
    fset,          // *token.FileSet
    funcMap,       // map[string]*ast.FuncDecl
)

// Write metadata to file
err := WriteMetadata(metadata, "output.yaml")

// Load metadata from file
loadedMeta, err := LoadMetadata("output.yaml")
```

## Best Practices

1. **Separation of Concerns**: Each file has a specific responsibility
2. **Reusable Components**: Functions are designed to be reusable and testable
3. **Error Handling**: Proper error handling throughout the codebase
4. **Documentation**: Comprehensive comments and documentation
5. **Type Safety**: Strong typing with Go's type system
6. **Performance**: Efficient algorithms and data structures
7. **Simplicity**: Clean, focused code without unnecessary complexity

## Maintenance

The modular structure makes it easy to:
- Add new expression types in `expression.go`
- Extend analysis capabilities in `analysis.go`
- Add new I/O formats in `io.go`
- Modify data structures in `types.go`
- Update core logic in `metadata.go`
- Add utility functions in `helpers.go`

Each file can be modified independently without affecting the others, as long as the interfaces remain consistent.

## Design Principles

- **Single Responsibility**: Each function and file has one clear purpose
- **Dependency Minimization**: Minimal dependencies between modules
- **Clean Interfaces**: Clear, well-documented function signatures
- **Error Handling**: Proper error propagation and handling
- **Performance**: Efficient algorithms and data structures
- **Maintainability**: Code that is easy to understand and modify 