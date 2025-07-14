# TrackerTree Usage Guide

## Overview

`TrackerTree` is a powerful data structure in the swagen project that provides efficient access to call graph information and enables sophisticated analysis of Go code relationships. This guide shows you how to use `TrackerTree` effectively instead of directly accessing metadata.

## Why Use TrackerTree?

1. **Efficient Function Lookup**: Find functions by name across all packages
2. **Call Chain Analysis**: Understand how functions are connected
3. **Route Pattern Detection**: Identify HTTP route registration patterns
4. **Dependency Analysis**: Find function dependencies and relationships
5. **Context-Aware Extraction**: Extract information with full call context

## Basic Usage

### Creating a TrackerTree

```mermaid
graph TD
  "github.com/ehabterra/swagen/testdata/chi/main" --> "github.com/ehabterra/swagen/testdata/chi/NewMux"
  "github.com/ehabterra/swagen/testdata/chi/NewMux"
  "github.com/ehabterra/swagen/testdata/chi/main" --> "github.com/ehabterra/swagen/testdata/chi/users/NewService"
  "github.com/ehabterra/swagen/testdata/chi/users/NewService"
  "github.com/ehabterra/swagen/testdata/chi/users/NewService" --> "github.com/ehabterra/swagen/testdata/chi/users/NewMux"
  "github.com/ehabterra/swagen/testdata/chi/users/NewMux"
  "github.com/ehabterra/swagen/testdata/chi/main" --> "github.com/go-chi/chi/v5/*chi.Mux.Mount"
  "github.com/go-chi/chi/v5/*chi.Mux.Mount"
  "github.com/go-chi/chi/v5/*chi.Mux.Mount" --> "github.com/ehabterra/swagen/testdata/chi/users/Routes"
  "github.com/ehabterra/swagen/testdata/chi/users/Routes"
  "github.com/ehabterra/swagen/testdata/chi/users/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get" --> "github.com/ehabterra/swagen/testdata/chi/users/ListUsers"
  "github.com/ehabterra/swagen/testdata/chi/users/ListUsers"
  "github.com/ehabterra/swagen/testdata/chi/users/ListUsers" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/users/ListUsers" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/users/ListUsers" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "[]github.com/ehabterra/swagen/testdata/chi/users.User.users"
  "[]github.com/ehabterra/swagen/testdata/chi/users.User.users"
  "github.com/ehabterra/swagen/testdata/chi/users/ListUsers" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/users/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get" --> "github.com/ehabterra/swagen/testdata/chi/users/GetUser"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser" --> "github.com/ehabterra/swagen/testdata/chi/users/URLParam"
  "github.com/ehabterra/swagen/testdata/chi/users/URLParam"
  "github.com/ehabterra/swagen/testdata/chi/users/URLParam" --> "*net/http.Request.r"
  "*net/http.Request.r"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "github.com/ehabterra/swagen/testdata/chi/users.User.user"
  "github.com/ehabterra/swagen/testdata/chi/users.User.user"
  "github.com/ehabterra/swagen/testdata/chi/users/GetUser" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/users/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Post"
  "github.com/go-chi/chi/v5/*chi.Mux.Post"
  "github.com/go-chi/chi/v5/*chi.Mux.Post" --> "github.com/ehabterra/swagen/testdata/chi/users/CreateUser"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "encoding/json/*json.Decoder.Decode"
  "encoding/json/*json.Decoder.Decode"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "encoding/json/NewDecoder"
  "encoding/json/NewDecoder"
  "encoding/json/NewDecoder" --> "*net/http.Request.r/Body"
  "*net/http.Request.r/Body"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "net/http/Error"
  "net/http/Error"
  "net/http/Error" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "net/http/Error" --> "github.com/ehabterra/swagen/testdata/chi/users/http/StatusBadRequest"
  "github.com/ehabterra/swagen/testdata/chi/users/http/StatusBadRequest"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "github.com/ehabterra/swagen/testdata/chi/users.User.user"
  "github.com/ehabterra/swagen/testdata/chi/users.User.user"
  "github.com/ehabterra/swagen/testdata/chi/users/CreateUser" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/main" --> "github.com/go-chi/chi/v5/*chi.Mux.Mount"
  "github.com/go-chi/chi/v5/*chi.Mux.Mount"
  "github.com/go-chi/chi/v5/*chi.Mux.Mount" --> "github.com/ehabterra/swagen/testdata/chi/products/Routes"
  "github.com/ehabterra/swagen/testdata/chi/products/Routes"
  "github.com/ehabterra/swagen/testdata/chi/products/Routes" --> "github.com/ehabterra/swagen/testdata/chi/products/NewRouter"
  "github.com/ehabterra/swagen/testdata/chi/products/NewRouter"
  "github.com/ehabterra/swagen/testdata/chi/products/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get" --> "untyped string.root"
  "untyped string.root"
  "github.com/go-chi/chi/v5/*chi.Mux.Get" --> "github.com/ehabterra/swagen/testdata/chi/products/ListProducts"
  "github.com/ehabterra/swagen/testdata/chi/products/ListProducts"
  "github.com/ehabterra/swagen/testdata/chi/products/ListProducts" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/products/ListProducts" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/products/ListProducts" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "[]github.com/ehabterra/swagen/testdata/chi/products.Product.products"
  "[]github.com/ehabterra/swagen/testdata/chi/products.Product.products"
  "github.com/ehabterra/swagen/testdata/chi/products/ListProducts" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/products/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Post"
  "github.com/go-chi/chi/v5/*chi.Mux.Post"
  "github.com/go-chi/chi/v5/*chi.Mux.Post" --> "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "encoding/json/*json.Decoder.Decode"
  "encoding/json/*json.Decoder.Decode"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "encoding/json/NewDecoder"
  "encoding/json/NewDecoder"
  "encoding/json/NewDecoder" --> "*net/http.Request.r/Body"
  "*net/http.Request.r/Body"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "net/http/Error"
  "net/http/Error"
  "net/http/Error" --> "github.com/ehabterra/swagen/testdata/chi/products/http/StatusBadRequest"
  "github.com/ehabterra/swagen/testdata/chi/products/http/StatusBadRequest"
  "net/http/Error" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "github.com/ehabterra/swagen/testdata/chi/products.Product.product"
  "github.com/ehabterra/swagen/testdata/chi/products.Product.product"
  "github.com/ehabterra/swagen/testdata/chi/products/CreateProduct" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/products/Routes" --> "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get"
  "github.com/go-chi/chi/v5/*chi.Mux.Get" --> "github.com/ehabterra/swagen/testdata/chi/products/GetProduct"
  "github.com/ehabterra/swagen/testdata/chi/products/GetProduct"
  "github.com/ehabterra/swagen/testdata/chi/products/GetProduct" --> "net/http/http.Header.Set"
  "net/http/http.Header.Set"
  "github.com/ehabterra/swagen/testdata/chi/products/GetProduct" --> "net/http/http.ResponseWriter.Header"
  "net/http/http.ResponseWriter.Header"
  "github.com/ehabterra/swagen/testdata/chi/products/GetProduct" --> "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode"
  "encoding/json/*json.Encoder.Encode" --> "github.com/ehabterra/swagen/testdata/chi/products.Product.product"
  "github.com/ehabterra/swagen/testdata/chi/products.Product.product"
  "github.com/ehabterra/swagen/testdata/chi/products/GetProduct" --> "encoding/json/NewEncoder"
  "encoding/json/NewEncoder"
  "encoding/json/NewEncoder" --> "net/http.ResponseWriter.w"
  "net/http.ResponseWriter.w"
  "github.com/ehabterra/swagen/testdata/chi/main" --> "net/http/ListenAndServe"
  "net/http/ListenAndServe"
  "net/http/ListenAndServe" --> "*github.com/go-chi/chi/v5.Mux.r"
  "*github.com/go-chi/chi/v5.Mux.r"
```


```go
// Create a TrackerTree from metadata
tree := NewTrackerTree(meta)
```

### Finding Functions

```go
// Find a function by name across all packages
fn, file, pkg := tree.FindFunctionByName("GetUser")
if fn != nil {
    fmt.Printf("Found function: %s in package %s\n", fn.Name, pkg)
}

// Find a function in a specific package
fn, file := tree.FindFunctionByPackageAndName("users", "GetUser")
```

### Getting Function Context

```go
// Get comprehensive context about a function
context := tree.GetFunctionContext("GetUser")

// Access callers and callees
if callers, ok := context["callers"].([]string); ok {
    for _, caller := range callers {
        fmt.Printf("Caller: %s\n", caller)
    }
}

if callees, ok := context["callees"].([]string); ok {
    for _, callee := range callees {
        fmt.Printf("Callee: %s\n", callee)
    }
}
```

## Advanced Analysis

### Route Pattern Analysis

```go
// Analyze the entire codebase for route registration patterns
patterns := tree.AnalyzeRoutePatterns()

for patternType, calls := range patterns {
    fmt.Printf("Pattern: %s\n", patternType)
    for _, call := range calls {
        fmt.Printf("  %s\n", call)
    }
}
```

### Finding Route Handlers

```go
// Find all functions that appear to be HTTP handlers
handlers := tree.FindRouteHandlers()
for _, handler := range handlers {
    fmt.Printf("Handler: %s\n", handler)
}
```

### Call Chain Analysis

```go
// Get the call chain leading to a specific function
callChain := tree.GetCallChainForFunction("GetUser")
for _, edge := range callChain {
    fmt.Printf("Call: %s -> %s\n", edge.Caller.Name, edge.Callee.Name)
}
```

### Dependency Analysis

```go
// Get all functions that a given function depends on
dependencies := tree.GetFunctionDependencies("GetUser")
for _, dep := range dependencies {
    fmt.Printf("Dependency: %s\n", dep)
}

// Get the call depth of a function
depth := tree.GetCallDepth("GetUser")
fmt.Printf("Call depth: %d\n", depth)
```

## TrackerTree-Aware Extraction Functions

Instead of using the basic extraction functions, use the TrackerTree-aware versions for better results:

### Request Body Extraction

```go
// Instead of: extractRequestBodyInfo(fn, file, cfg, meta)
// Use:
requestBodyType, hasRequestBody := extractRequestBodyInfoWithTrackerTree(tree, fn, file, cfg)
```

### Response Extraction

```go
// Instead of: extractResponseInfo(fn, file, cfg, meta)
// Use:
responseType, statusCode, hasResponse := extractResponseInfoWithTrackerTree(tree, fn, file, cfg)
```

### Parameter Extraction

```go
// Instead of: extractParamInfo(fn, file, cfg, meta)
// Use:
params := extractParamInfoWithTrackerTree(tree, fn, file, cfg)
```

### Route Information Extraction

```go
// Instead of: extractRouteInfo(fn, file, cfg, meta, funcMap)
// Use:
path, method, handler := extractRouteInfoWithTrackerTree(tree, fn, file, cfg)
```

## Enhanced Route Collection

Use the enhanced route collection function that leverages TrackerTree:

```go
// Instead of manually collecting routes, use:
routes := EnhancedRouteCollection(meta, cfg)
```

This function:
1. Uses TrackerTree to find all potential handlers
2. Analyzes call chains to infer route information
3. Uses TrackerTree-aware extraction functions
4. Provides better context for route analysis

## Best Practices

### 1. Always Use TrackerTree for Function Lookup

```go
// Good: Use TrackerTree
fn, file, pkg := tree.FindFunctionByName("GetUser")

// Avoid: Direct metadata access
for pkgName, pkg := range meta.Packages {
    for _, file := range pkg.Files {
        if fn, exists := file.Functions["GetUser"]; exists {
            // Found it
        }
    }
}
```

### 2. Use Context-Aware Extraction

```go
// Good: Get function context first
context := tree.GetFunctionContext("GetUser")
if callChain, ok := context["call_chain"].([]*metadata.CallGraphEdge); ok {
    // Use call chain information for better extraction
}

// Then use TrackerTree-aware extraction functions
requestBodyType, hasRequestBody := extractRequestBodyInfoWithTrackerTree(tree, fn, file, cfg)
```

### 3. Leverage Pattern Analysis

```go
// Analyze patterns before processing
patterns := tree.AnalyzeRoutePatterns()
handlers := tree.FindRouteHandlers()

// Use this information to guide your processing
for _, handler := range handlers {
    // Process each handler with full context
}
```

### 4. Use Call Chain Information

```go
// Get call chain for better understanding
callChain := tree.GetCallChainForFunction("GetUser")
for _, edge := range callChain {
    // Analyze each call in the chain
    // Extract arguments, understand relationships
}
```

## Example: Complete Route Analysis

```go
func analyzeRoutesWithTrackerTree(meta *metadata.Metadata, cfg *SwagenConfig) {
    tree := NewTrackerTree(meta)
    
    // 1. Find all potential handlers
    handlers := tree.FindRouteHandlers()
    
    // 2. Analyze patterns
    patterns := tree.AnalyzeRoutePatterns()
    
    // 3. Process each handler
    for _, handlerName := range handlers {
        fn, file, _ := tree.FindFunctionByName(handlerName)
        if fn == nil {
            continue
        }
        
        // 4. Get full context
        context := tree.GetFunctionContext(handlerName)
        
        // 5. Extract route information with context
        path, method, _ := extractRouteInfoWithTrackerTree(tree, fn, file, cfg)
        
        // 6. Extract operation details
        requestBodyType, hasRequestBody := extractRequestBodyInfoWithTrackerTree(tree, fn, file, cfg)
        responseType, statusCode, hasResponse := extractResponseInfoWithTrackerTree(tree, fn, file, cfg)
        params := extractParamInfoWithTrackerTree(tree, fn, file, cfg)
        
        // 7. Build operation with full context
        op := buildOperationFromFunction(fn, file, cfg, meta, 
            requestBodyType, hasRequestBody, responseType, statusCode, hasResponse, params)
        
        fmt.Printf("Route: %s %s -> %s\n", method, path, handlerName)
    }
}
```

## Performance Benefits

Using TrackerTree provides several performance benefits:

1. **Efficient Lookups**: O(1) function lookups instead of O(n) searches
2. **Cached Relationships**: Call relationships are pre-computed
3. **Reduced Redundancy**: Avoid repeated metadata traversal
4. **Better Context**: Full call chain information available immediately

## Migration Guide

To migrate existing code to use TrackerTree:

1. **Replace direct metadata access** with TrackerTree methods
2. **Use TrackerTree-aware extraction functions** instead of basic ones
3. **Leverage context information** for better analysis
4. **Use pattern analysis** to understand code structure

## Conclusion

TrackerTree provides a powerful, efficient way to analyze Go code relationships and extract OpenAPI specifications. By using TrackerTree consistently throughout your code, you'll get better results, improved performance, and more comprehensive analysis of your codebase. 