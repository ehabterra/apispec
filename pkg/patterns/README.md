# Pattern Matching Package

A high-performance, gitignore-style pattern matching library for Go that supports recursive directory matching and complex glob patterns.

## Features

- ✅ **Gitignore Compatibility**: Full support for `.gitignore` pattern syntax
- ✅ **Recursive Matching**: `**` patterns for deep directory traversal  
- ✅ **Character Classes**: `[0-9]`, `[a-z]`, `[abc]` patterns
- ✅ **Wildcards**: `*` (any chars except `/`) and `?` (single char)
- ✅ **Negation**: `!pattern` for exclusion patterns
- ✅ **Directory Patterns**: `dir/` matches directories and contents
- ✅ **High Performance**: Optimized regex-based implementation
- ✅ **Zero Dependencies**: Only uses Go standard library

## Quick Start

```go
import "github.com/ehabterra/apispec/pkg/patterns"

// Simple pattern matching
match := patterns.Match("*.go", "main.go")                    // true
match = patterns.Match("**/*.go", "src/internal/service.go")  // true

// Match against multiple patterns
patterns := []string{"*.go", "*.txt", "**/*.md"}
match = patterns.MatchAny(patterns, "docs/readme.md")         // true

// Filter lists with include/exclude patterns
paths := []string{"main.go", "test.go", "readme.txt"}
filtered := patterns.Filter(paths, []string{"*.go"}, []string{"*test*"})
// Result: ["main.go"]
```

## Supported Pattern Syntax

| Pattern | Description | Example | Matches |
|---------|-------------|---------|---------|
| `*` | Any characters except `/` | `*.go` | `main.go`, `util.go` |
| `**` | Zero or more directories | `**/*.go` | `main.go`, `src/main.go` |
| `?` | Single character except `/` | `test?.go` | `test1.go`, `testa.go` |
| `[...]` | Character class | `test[0-9].go` | `test1.go`, `test9.go` |
| `!` | Negation (exclude) | `!*.tmp` | Excludes `.tmp` files |
| `dir/` | Directory pattern | `src/` | `src/main.go`, `src/` |

## Performance

The package is optimized for common use cases:

| Pattern Type | Performance | Use Case |
|-------------|-------------|----------|
| Simple (`*.go`) | ~6µs | File extensions |
| Single Star (`src/*.go`) | ~9µs | Single directory |
| Double Star (`**/*.go`) | ~15µs | Recursive search |
| Complex (`org/**/internal/**`) | ~21µs | Complex filtering |

### Performance Tools

Use the included performance testing tool:

```bash
# Quick benchmark with common patterns
go run cmd/perf-test/main.go -quick

# Compare multiple patterns
go run cmd/perf-test/main.go -compare "*.go,**/*.go,src/**/*.go" -path "src/main.go"

# Detailed profiling
go run cmd/perf-test/main.go -pattern "**/*.go" -path "src/main.go" -profile
```

## API Reference

### Core Functions

#### `Match(pattern, path string) bool`
Tests if a path matches a gitignore-style pattern.

```go
patterns.Match("*.go", "main.go")                    // true
patterns.Match("src/**/*.go", "src/pkg/util.go")     // true  
patterns.Match("!*.tmp", "file.go")                  // true (negation)
```

#### `MatchAny(patterns []string, path string) bool`
Tests if a path matches any of the given patterns.

```go
patterns := []string{"*.go", "*.txt"}
patterns.MatchAny(patterns, "main.go")               // true
```

#### `Filter(paths, includePatterns, excludePatterns []string) []string`
Filters a list of paths using include and exclude patterns.

```go
paths := []string{"main.go", "test.go", "readme.txt"}
filtered := patterns.Filter(paths, []string{"*.go"}, []string{"*test*"})
// Returns: ["main.go"]
```

### Performance Tools

#### `BenchmarkPattern(pattern, path string, iterations int) PerfResult`
Measures performance of a single pattern.

#### `QuickBench()`
Runs a quick performance test with common patterns.

#### `ProfilePattern(pattern, path string, iterations int)`
Provides detailed profiling information.

## Real-World Examples

### API Package Filtering
```go
// Include all modules under a specific organization
includePatterns := []string{
    "github.com/myorg/project/modules/**",
    "github.com/myorg/project/services/**",
}

// Exclude test and internal packages
excludePatterns := []string{
    "**/test/**",
    "**/internal/**", 
    "**/*_test.go",
}

filtered := patterns.Filter(allPackages, includePatterns, excludePatterns)
```

### File Type Filtering
```go
// Include source files, exclude temporary files
sourcePatterns := []string{"**/*.go", "**/*.proto", "**/*.sql"}
excludePatterns := []string{"**/*.tmp", "**/*.log", "**/vendor/**"}

sourceFiles := patterns.Filter(allFiles, sourcePatterns, excludePatterns)
```

### Monorepo Navigation
```go
// Match specific services in a monorepo
servicePatterns := []string{
    "services/auth/**",
    "services/user/**", 
    "services/payment/**",
}

match := patterns.MatchAny(servicePatterns, "services/auth/handler.go") // true
```

## Benchmarks

Run comprehensive benchmarks:

```bash
go test ./pkg/patterns -bench=. -benchmem
```

Key performance metrics on Intel i9-9880H:
- **Simple patterns**: ~6µs, 39 allocs
- **Complex patterns**: ~21µs, 110 allocs  
- **Large dataset filtering**: ~35ms for 1000 items

## Contributing

1. Run tests: `go test ./pkg/patterns`
2. Run benchmarks: `go test ./pkg/patterns -bench=.`
3. Check performance: `go run cmd/perf-test/main.go -quick`

## License

Same as parent project - Apache License 2.0.
