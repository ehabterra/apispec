# APISpec CLI Tool

The APISpec CLI tool is the main command-line interface for generating OpenAPI specifications from Go code. It analyzes your Go project, detects web frameworks, and generates comprehensive OpenAPI 3.1 specifications.

## Features

- **Automated OpenAPI Generation**: Generate OpenAPI 3.1 specs from real Go code
- **Framework Detection**: Automatically detects Gin, Echo, Chi, Fiber, and net/http
- **Call Graph Analysis**: Builds call graphs to resolve handlers, parameters, and responses
- **Smart Type Resolution**: Resolves aliases, enums, and generic types
- **Validator Tag Support**: Comprehensive support for go-playground/validator tags
- **Performance Profiling**: Built-in CPU, memory, and trace profiling
- **Configurable Limits**: Fine-tune analysis for large codebases

## Quick Start

### Installation

```bash
# Install via Go
go install github.com/ehabterra/apispec/cmd/apispec@latest

# Or build from source
git clone https://github.com/ehabterra/apispec.git
cd apispec
make build
```

### Basic Usage

```bash
# Generate OpenAPI spec from current directory
./apispec --output openapi.yaml

# Generate with call graph diagram
./apispec --output openapi.yaml --diagram

# Generate with custom configuration
./apispec --config my-config.yaml --output openapi.yaml

# Performance profiling for large projects
./apispec --output openapi.yaml --cpu-profile --mem-profile
```

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--output`, `-o` | Output file for OpenAPI spec | `openapi.json` |
| `--dir`, `-d` | Directory to parse for Go files | `.` (current dir) |
| `--config`, `-c` | Path to custom config YAML | `""` |
| `--diagram`, `-g` | Save call graph as HTML | `""` |
| `--write-metadata`, `-w` | Write metadata.yaml to disk | `false` |
| `--version`, `-V` | Show version information | `false` |
| `--cpu-profile` | Enable CPU profiling | `false` |
| `--mem-profile` | Enable memory profiling | `false` |
| `--skip-cgo` | Skip CGO packages during analysis | `true` |

## Examples

```bash
# Basic OpenAPI generation
./apispec --output openapi.yaml

# Generate with diagram and metadata
./apispec --output openapi.yaml --diagram --write-metadata

# Analyze specific directory with custom limits
./apispec --dir ./myproject --output openapi.yaml --max-nodes 100000

# Performance analysis
./apispec --output openapi.yaml --cpu-profile --mem-profile --trace-profile

# Show version information
./apispec --version
```

## Configuration

APISpec uses YAML configuration files to define framework patterns and behavior. See the main project documentation for detailed configuration examples.

## Framework Support

- **Gin**: Full support for route registration and parameter handling
- **Chi**: Full support for route mounting, grouping, and parameter extraction
- **Echo**: Full support for route registration, grouping, and parameter handling
- **Fiber**: Full support for route registration, grouping, and parameter handling
- **Gorilla Mux**: Route registration and handler detection
- **Standard net/http**: Basic support for HandleFunc and Handle calls

## Output Formats

- **JSON**: `openapi.json` (default)
- **YAML**: `openapi.yaml`
- **HTML Diagram**: Interactive call graph visualization
- **Metadata**: Detailed analysis data in YAML format

## Performance Considerations

For large codebases, consider these optimizations:

- Use `--max-nodes` to limit call graph size
- Use `--max-recursion-depth` to prevent infinite loops
- Enable `--skip-cgo` to avoid CGO build issues
- Use profiling flags to identify bottlenecks

## Troubleshooting

### Common Issues

1. **Build errors**: Use `--skip-cgo` to skip problematic CGO packages
2. **Memory issues**: Reduce `--max-nodes` and `--max-children` limits
3. **Slow performance**: Enable profiling to identify bottlenecks
4. **Empty output**: Check that your project contains web framework code

### Debug Mode

```bash
# Generate metadata for debugging
./apispec --output openapi.yaml --write-metadata

# Enable profiling
./apispec --output openapi.yaml --cpu-profile --mem-profile
```

## Related Tools

- **[apidiag](cmd/apidiag/README.md)**: Interactive web-based diagram server
- **[Main Documentation](../../README.md)**: Complete project documentation

## License

Apache License 2.0 - See [LICENSE](../../LICENSE) for details.
