# Interface Resolution in Swagen

## Overview

Swagen now includes a minimal interface resolution system that allows you to resolve embedded interfaces in Go structs to their concrete implementations. This is particularly useful for handling dependency injection patterns like those found in go-clean-echo and similar frameworks.

## The Problem

Go's interface embedding creates a "flattened" method set where interface methods become part of the parent struct. However, when analyzing code for OpenAPI generation, you need to know the actual concrete types to generate accurate schemas.

### Example Pattern
```go
type Handlers struct {
    AuthorHandler  // embedded interface
    BookHandler   // embedded interface
}

func New(s *services.Services) *Handlers {
    return &Handlers{
        AuthorHandler: &authorHandler{s.Author},  // concrete implementation
        BookHandler:   &bookHandler{s.Book},      // concrete implementation
    }
}
```

In this case:
- `Handlers.GetAuthors` exists at compile time
- But you need to know it maps to `authorHandler.GetAuthors`
- The interface type information is "erased" during embedding

## Solution

The interface resolution system provides a simple registry to map interface types to concrete implementations in specific struct contexts.

## Usage

### 1. Register Interface Resolutions

During your code analysis, register interface resolutions when you discover them:

```go
// Create tracker tree
tree := NewTrackerTree(meta, limits)

// Register interface resolutions discovered during analysis
tree.RegisterInterfaceResolution(
    "AuthorHandler",           // Interface type
    "Handlers",                // Struct type containing the embedded interface
    "github.com/example/app",  // Package where Handlers is defined
    "*authorHandler",          // Concrete implementation type
)

tree.RegisterInterfaceResolution(
    "BookHandler",             // Interface type
    "Handlers",                // Struct type containing the embedded interface
    "github.com/example/app",  // Package where Handlers is defined
    "*bookHandler",            // Concrete implementation type
)
```

### 2. Resolve Interfaces

When you need to resolve an interface method call:

```go
// Resolve Handlers.GetAuthors -> authorHandler.GetAuthors
concreteType := tree.ResolveInterface("AuthorHandler", "Handlers", "github.com/example/app")
// concreteType will be "*authorHandler"

// Use this concrete type for further analysis, schema generation, etc.
```

### 3. Debug Interface Resolutions

You can inspect all registered interface resolutions:

```go
resolutions := tree.GetInterfaceResolutions()
for key, concreteType := range resolutions {
    fmt.Printf("Interface %s in struct %s (%s) -> %s\n", 
        key.InterfaceType, key.StructType, key.Pkg, concreteType)
}
```

## API Reference

### TrackerTree Methods

#### `RegisterInterfaceResolution(interfaceType, structType, pkg, concreteType string)`
Registers a mapping from an interface type to its concrete implementation in a specific struct context.

**Parameters:**
- `interfaceType`: The interface type name (e.g., "AuthorHandler")
- `structType`: The struct type containing the embedded interface (e.g., "Handlers")
- `pkg`: Package where the struct is defined (e.g., "github.com/example/app")
- `concreteType`: The concrete implementation type (e.g., "*authorHandler")

#### `ResolveInterface(interfaceType, structType, pkg string) string`
Resolves an interface type to its concrete implementation in a struct context.

**Parameters:**
- `interfaceType`: The interface type name
- `structType`: The struct type containing the embedded interface
- `pkg`: Package where the struct is defined

**Returns:**
- The concrete type if found
- The original interface type if no resolution exists

#### `GetInterfaceResolutions() map[interfaceKey]string`
Returns all registered interface resolutions for debugging purposes.

## Integration with Existing Code

The interface resolution system is designed to integrate seamlessly with Swagen's existing architecture:

1. **Minimal Changes**: Only adds a new map and three methods to TrackerTree
2. **No Breaking Changes**: All existing functionality remains unchanged
3. **Performance**: Simple map lookups with no reflection overhead
4. **Extensible**: Can be enhanced to handle more complex patterns

## When to Use

Use interface resolution when you encounter:

- **Embedded interfaces** in structs
- **Dependency injection** patterns
- **Interface flattening** that hides concrete types
- **Method dispatch** that needs concrete type information

## Example Integration

Here's how you might integrate this into your existing Swagen analysis:

```go
// In your existing analysis code
func analyzeStructField(field *ast.Field, tree *TrackerTree, pkg string) {
    if field.Type != nil {
        // Check if this is an embedded interface
        if ident, ok := field.Type.(*ast.Ident); ok {
            if isInterface(ident.Name) {
                // This is an embedded interface
                structType := getCurrentStructName()
                concreteType := findConcreteImplementation(field, tree)
                
                if concreteType != "" {
                    tree.RegisterInterfaceResolution(
                        ident.Name,      // interface type
                        structType,      // struct type
                        pkg,            // package
                        concreteType,    // concrete implementation
                    )
                }
            }
        }
    }
}

// Later, when you need to resolve a method call
func resolveMethodCall(receiver, method string, tree *TrackerTree, pkg string) string {
    // Try to resolve interface to concrete type
    concreteType := tree.ResolveInterface(receiver, "Handlers", pkg)
    
    if concreteType != receiver {
        // Interface was resolved, use concrete type
        return concreteType + "." + method
    }
    
    // No resolution, use original
    return receiver + "." + method
}
```

## Benefits

1. **Accurate Schema Generation**: Generate OpenAPI specs based on concrete types, not interfaces
2. **Better Type Resolution**: Follow method calls to actual implementations
3. **Dependency Injection Support**: Handle common Go patterns for web frameworks
4. **Minimal Overhead**: Simple map-based lookup system
5. **Easy Integration**: Drop-in addition to existing TrackerTree functionality

## Limitations

1. **Manual Registration**: Interface resolutions must be discovered and registered during analysis
2. **Package-Specific**: Resolutions are tied to specific package contexts
3. **No Automatic Discovery**: The system doesn't automatically detect interface implementations

## Future Enhancements

Potential future improvements could include:

1. **Automatic Discovery**: Automatically detect interface implementations during AST analysis
2. **Pattern Matching**: Use regex patterns to match interface names to implementation names
3. **Cross-Package Resolution**: Handle interface implementations across package boundaries
4. **Generic Support**: Handle generic interface implementations
5. **Method-Level Resolution**: Resolve specific methods, not just entire interfaces
