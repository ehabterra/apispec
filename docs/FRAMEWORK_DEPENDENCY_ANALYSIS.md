# Framework Dependency Analysis

## Overview

The Framework Dependency Analysis feature allows you to collect all packages that have dependencies on framework imports and build their complete dependency tree. This is crucial for understanding which packages are framework-related and should be included in your OpenAPI generation.

## Features

### ✅ **Comprehensive Framework Detection**
- **Gin**: `github.com/gin-gonic/gin`, `github.com/gin-contrib/*`
- **Echo**: `github.com/labstack/echo`, `github.com/labstack/echo/v4`
- **Fiber**: `github.com/gofiber/fiber`, `github.com/gofiber/fiber/v2`
- **Chi**: `github.com/go-chi/chi`, `github.com/go-chi/chi/v5`
- **Mux**: `github.com/gorilla/mux`
- **HTTP**: `net/http`
- **FastHTTP**: `github.com/valyala/fasthttp`

### ✅ **Dependency Tree Analysis**
- **Root Packages**: Direct framework users
- **Dependency Chains**: Complete dependency relationships
- **Dependent Packages**: Packages that depend on framework packages
- **Dependency Depth**: How deep the dependency chain goes

### ✅ **Package Content Analysis**
- **Files**: All Go files in each package
- **Functions**: All functions defined in each package
- **Types**: All types defined in each package
- **Metadata**: Package statistics and information

## Usage

### 1. Enable Framework Dependency Analysis

```go
// Create engine configuration
cfg := engine.DefaultEngineConfig()
cfg.AnalyzeFrameworkDependencies = true // Enable analysis
cfg.ModuleRoot = "." // Your project root

// Create engine
eng, err := engine.NewEngine(cfg)
if err != nil {
    log.Fatal(err)
}
```

### 2. Generate OpenAPI with Analysis

```go
// Generate OpenAPI spec (includes dependency analysis)
spec, err := eng.GenerateOpenAPI(".")
if err != nil {
    log.Fatal(err)
}

// Get metadata with dependency tree
meta := eng.GetMetadata()
if meta.FrameworkDependencyTree != nil {
    // Analysis completed successfully
    fmt.Printf("Found %d framework packages\n", meta.FrameworkDependencyTree.TotalPackages)
}
```

### 3. Access Dependency Information

```go
// Get the dependency tree
tree := meta.FrameworkDependencyTree

// Print complete dependency tree
tree.PrintDependencyTree()

// Get packages grouped by framework type
frameworkPackages := tree.GetFrameworkPackages()
for frameworkType, packages := range frameworkPackages {
    fmt.Printf("%s: %d packages\n", frameworkType, len(packages))
}

// Get dependency chain for a specific package
chain := tree.GetDependencyChain("github.com/yourproject/api")
for _, dep := range chain {
    fmt.Printf("  %s (depth: %d)\n", dep.PackagePath, dep.Depth)
}
```

## Data Structures

### FrameworkDependency

```go
type FrameworkDependency struct {
    PackagePath    string                    `yaml:"package_path"`
    FrameworkType  string                    `yaml:"framework_type"`
    ImportPath     string                    `yaml:"import_path"`
    Dependencies   []string                  `yaml:"dependencies"`
    Dependents     []string                  `yaml:"dependents"`
    IsDirect       bool                      `yaml:"is_direct"`
    Depth          int                       `yaml:"depth"`
    Files          []string                  `yaml:"files"`
    Functions      []string                  `yaml:"functions"`
    Types          []string                  `yaml:"types"`
    Metadata       map[string]interface{}    `yaml:"metadata,omitempty"`
}
```

### FrameworkDependencyTree

```go
type FrameworkDependencyTree struct {
    RootPackages   []string                           `yaml:"root_packages"`
    AllPackages    map[string]*FrameworkDependency    `yaml:"all_packages"`
    FrameworkTypes map[string][]string                `yaml:"framework_types"`
    TotalPackages  int                                `yaml:"total_packages"`
    MaxDepth       int                                `yaml:"max_depth"`
}
```

## Example Output

```
Framework Dependency Tree
========================
Total Packages: 15
Max Depth: 3
Root Packages: 3

GIN Framework (2 packages):
  github.com/yourproject/api (depth: 0, files: 3, functions: 12)
  github.com/yourproject/handlers (depth: 0, files: 5, functions: 8)

ECHO Framework (1 packages):
  github.com/yourproject/legacy (depth: 0, files: 2, functions: 4)

Dependency Chains:
  github.com/yourproject/api (gin, depth: 0)
    github.com/yourproject/models (dependent, depth: 1)
      github.com/yourproject/database (dependency, depth: 2)
    github.com/yourproject/middleware (dependent, depth: 1)
  github.com/yourproject/handlers (gin, depth: 0)
    github.com/yourproject/models (dependent, depth: 1)
  github.com/yourproject/legacy (echo, depth: 0)
    github.com/yourproject/oldmodels (dependent, depth: 1)
```

## Use Cases

### 1. **OpenAPI Generation Scope**
```go
// Only generate OpenAPI for framework-related packages
frameworkPackages := tree.GetFrameworkPackages()
for frameworkType, packages := range frameworkPackages {
    fmt.Printf("Generating OpenAPI for %s packages...\n", frameworkType)
    for _, pkg := range packages {
        // Generate OpenAPI for this package
        generateOpenAPIForPackage(pkg.PackagePath)
    }
}
```

### 2. **Dependency Visualization**
```go
// Create a dependency graph visualization
for _, rootPkg := range tree.RootPackages {
    chain := tree.GetDependencyChain(rootPkg)
    createDependencyGraph(chain)
}
```

### 3. **Framework Migration Analysis**
```go
// Analyze which packages need to be migrated
ginPackages := tree.FrameworkTypes["gin"]
echoPackages := tree.FrameworkTypes["echo"]

fmt.Printf("Gin packages to migrate: %d\n", len(ginPackages))
fmt.Printf("Echo packages to migrate: %d\n", len(echoPackages))
```

### 4. **Package Filtering**
```go
// Filter packages for analysis
var frameworkRelatedPackages []string
for _, dep := range tree.AllPackages {
    if dep.FrameworkType != "dependent" && dep.FrameworkType != "dependency" {
        frameworkRelatedPackages = append(frameworkRelatedPackages, dep.PackagePath)
    }
}
```

## Configuration Options

### EngineConfig

```go
type EngineConfig struct {
    // ... other fields ...
    AnalyzeFrameworkDependencies bool // Enable/disable analysis
    AutoIncludeFrameworkPackages bool // Auto-populate IncludePackages with framework packages
}
```

### Default Configuration

```go
func DefaultEngineConfig() *EngineConfig {
    return &EngineConfig{
        // ... other defaults ...
        AnalyzeFrameworkDependencies: false, // Default to false for performance
        AutoIncludeFrameworkPackages: false, // Default to false for explicit control
    }
}
```

## Performance Considerations

### ✅ **Optimized Analysis**
- **Lazy Loading**: Only analyzes when enabled
- **Efficient Traversal**: Uses maps for O(1) lookups
- **Cycle Detection**: Prevents infinite loops in dependency chains
- **Memory Efficient**: Reuses existing package information

### ⚠️ **Performance Impact**
- **Analysis Time**: Adds ~10-20% to total generation time
- **Memory Usage**: Stores additional dependency information
- **Default**: Disabled by default for performance

## Advanced Usage

### 1. **Custom Framework Detection**

```go
// Create custom framework detector
detector := metadata.NewFrameworkDetector()

// Add custom framework patterns
detector.FrameworkPatterns["custom"] = []string{
    "github.com/yourcompany/custom-framework",
    "github.com/yourcompany/custom-middleware",
}

// Analyze dependencies
tree, err := detector.AnalyzeFrameworkDependencies(pkgs, pkgsMetadata, fileToInfo, fset)
```

### 2. **Dependency Chain Analysis**

```go
// Find all packages at a specific depth
var packagesAtDepth2 []*metadata.FrameworkDependency
for _, dep := range tree.AllPackages {
    if dep.Depth == 2 {
        packagesAtDepth2 = append(packagesAtDepth2, dep)
    }
}
```

### 3. **Framework Type Statistics**

```go
// Get statistics for each framework type
for frameworkType, packages := range tree.FrameworkTypes {
    totalFunctions := 0
    totalFiles := 0
    
    for _, pkgPath := range packages {
        if dep, exists := tree.AllPackages[pkgPath]; exists {
            totalFunctions += len(dep.Functions)
            totalFiles += len(dep.Files)
        }
    }
    
    fmt.Printf("%s: %d packages, %d functions, %d files\n", 
        frameworkType, len(packages), totalFunctions, totalFiles)
}
```

## Auto-Include Framework Packages

### Overview

The `AutoIncludeFrameworkPackages` feature automatically populates the `IncludePackages` configuration with all framework-related packages found during dependency analysis. This allows you to focus your OpenAPI generation on framework-relevant code without manual configuration.

### Configuration

```go
cfg := engine.DefaultEngineConfig()
cfg.AnalyzeFrameworkDependencies = true
cfg.AutoIncludeFrameworkPackages = true // Automatically add framework packages to IncludePackages
```

### How It Works

1. **Analysis Phase**: Framework dependencies are analyzed
2. **Auto-Inclusion**: All framework packages (direct + indirect) are added to `IncludePackages`
3. **Import Analysis**: All packages imported by framework packages are discovered and included
4. **Package Filtering**: Packages are filtered to include framework packages + their imports
5. **Metadata Generation**: Metadata is generated only on the filtered package set (performance optimization!)
6. **Preservation**: Existing `IncludePackages` are preserved (no duplicates)
7. **OpenAPI Generation**: Framework packages and their imports are included in the final OpenAPI spec

### Example Usage

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/ehabterra/apispec/internal/engine"
)

func main() {
    // Create engine configuration
    cfg := engine.DefaultEngineConfig()
    cfg.AnalyzeFrameworkDependencies = true
    cfg.AutoIncludeFrameworkPackages = true
    
    // You can also manually add some packages
    cfg.IncludePackages = []string{
        "github.com/yourproject/shared", // This will be preserved
    }
    
    fmt.Printf("Initial IncludePackages: %v\n", cfg.IncludePackages)
    
    // Create engine
    eng := engine.NewEngine(cfg)
    
    // Generate OpenAPI spec (analyzes dependencies and auto-includes framework packages)
    _, err := eng.GenerateOpenAPI()
    if err != nil {
        log.Fatal(err)
    }
    
    // Show the final IncludePackages configuration
    finalConfig := eng.GetConfig()
    fmt.Printf("Final IncludePackages: %v\n", finalConfig.IncludePackages)
    
    // Get framework dependency information
    meta := eng.GetMetadata()
    if meta.FrameworkDependencyList != nil {
        fmt.Printf("Found %d framework packages\n", meta.FrameworkDependencyList.TotalPackages)
    }
}
```

### Imported Packages Analysis

When `AutoIncludeFrameworkPackages=true`, the system automatically includes:

1. **Framework Packages**: Direct framework packages (e.g., `github.com/gin-gonic/gin`)
2. **Framework Dependents**: Packages that use frameworks (e.g., `github.com/yourproject/api`)
3. **Imported Packages**: Project-related packages that framework packages import (e.g., `github.com/yourproject/models`, `github.com/yourproject/database`)

**Smart Filtering:**
- **Excludes External Packages**: Skips external dependencies like `golang.org/x/`, `google.golang.org/`, etc.
- **Includes Project Packages**: Only includes packages that belong to your project (contain `/modules/`, `/pkg/`, `/internal/`, etc.)
- **Recursive Analysis**: Finds imports of imports (up to 2 levels deep by default)
- **Depth Control**: Prevents infinite recursion and overly broad inclusion

**Example:**
```
Framework Package: github.com/yourproject/api (uses Gin)
├── Imports: github.com/gin-gonic/gin (EXTERNAL - excluded)
├── Imports: github.com/yourproject/models (PROJECT - included)
│   ├── Imports: github.com/yourproject/domain (PROJECT - included)
│   └── Imports: golang.org/x/crypto (EXTERNAL - excluded)
├── Imports: github.com/yourproject/database (PROJECT - included)
└── Imports: github.com/yourproject/utils (PROJECT - included)

Result: Only 4 project packages are included in metadata generation
```

### Benefits

- **Automatic Focus**: No need to manually specify framework packages
- **Comprehensive Coverage**: Includes both direct and indirect framework dependencies
- **Smart Filtering**: Only includes project-related packages, excludes external dependencies
- **Recursive Analysis**: Finds imports of imports to ensure complete coverage
- **Depth Control**: Prevents overly broad inclusion with configurable depth limits
- **Preserves Manual Configuration**: Existing `IncludePackages` are not overwritten
- **Intelligent Filtering**: Framework packages and their project imports are included in OpenAPI generation
- **Performance Optimization**: Metadata generation only works on the filtered package set, making the process much faster

## Integration with OpenAPI Generation

The framework dependency analysis integrates seamlessly with your existing OpenAPI generation workflow:

1. **Enable Analysis**: Set `AnalyzeFrameworkDependencies: true`
2. **Enable Auto-Include**: Set `AutoIncludeFrameworkPackages: true` (optional)
3. **Generate OpenAPI**: Call `eng.GenerateOpenAPI()`
4. **Access Results**: Get dependency list from `eng.GetMetadata()`
5. **Focused Generation**: OpenAPI is generated only for framework-related packages

## Conclusion

The Framework Dependency Analysis feature provides comprehensive insights into your project's framework usage and dependencies. It's essential for:

- **Understanding Project Structure**: See which packages are framework-related
- **Optimizing OpenAPI Generation**: Focus on relevant packages
- **Migration Planning**: Identify packages that need framework updates
- **Dependency Management**: Understand the impact of framework changes

This feature makes your OpenAPI generation more intelligent and targeted! 🎯
