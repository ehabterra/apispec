# APISpec API Diagram Server (apidiag)

APISpec API Diagram Server (apidiag) is a web-based tool that provides an interactive, paginated visualization of Go project call graphs. It analyzes your Go codebase and serves an interactive diagram through a web interface, allowing you to explore function relationships, dependencies, and code structure in real-time.

## Demo Video

[![APISpec API Diagram Server Demo](https://img.youtube.com/vi/UshBJ5-ayzA/maxresdefault.jpg)](https://youtu.be/UshBJ5-ayzA)

Click *the image above to watch the full demo on YouTube*

- Complete walkthrough of the diagram server
- Advanced filtering examples
- Export functionality demonstration

## Features

- **Interactive Web Interface**: Browse call graphs through a modern web UI
- **Paginated Visualization**: Handle large codebases with efficient pagination
- **Advanced Filtering**: Filter by packages, functions, files, receivers, signatures, and more
- **Real-time Analysis**: Live analysis of your Go project structure
- **Export Capabilities**: Export diagrams in multiple formats (SVG, PNG, PDF, JSON)
- **Performance Optimized**: Built-in caching and depth limiting for large projects
- **RESTful API**: Programmatic access to diagram data via HTTP API

## Installation

### Prerequisites

- Go 1.24 or later
- A Go project to analyze

### Build from Source

```bash
# Clone the repository
git clone https://github.com/ehabterra/apispec.git
cd apispec

# Build the API diagram server
go build -o apidiag ./cmd/apidiag

# Or build with make
make build-apidiag
```

### Install Globally

```bash
# Install to your Go bin directory
go install github.com/ehabterra/apispec/cmd/apidiag@latest

# Make sure your Go bin is in PATH
export PATH=$HOME/go/bin:$PATH
```

## Usage

### Basic Usage

```bash
# Start the server in your Go project directory
./apidiag

# Or if installed globally
apidiag

# The server will start on http://localhost:8080
```

### Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | Server port | `8080` |
| `--host` | Server host | `localhost` |
| `--dir` | Input directory containing Go source files | `.` (current directory) |
| `--page-size` | Default page size for pagination | `100` |
| `--max-depth` | Maximum call graph depth | `3` |
| `--cors` | Enable CORS headers | `true` |
| `--cache-timeout` | Cache timeout for metadata | `5m` |
| `--static` | Directory to serve static files from | `""` |
| `--verbose` | Enable verbose logging | `false` |
| `--version` | Show version information | `false` |
| `--auto-exclude-tests` | Auto-exclude test files | `true` |
| `--auto-exclude-mocks` | Auto-exclude mock files | `true` |

### Examples

```bash
# Start server on custom port
./apidiag --port 9090

# Analyze specific directory
./apidiag --dir ./my-go-project

# Custom page size and depth
./apidiag --page-size 50 --max-depth 2

# Serve static files alongside the diagram
./apidiag --static ./public

# Enable verbose logging
./apidiag --verbose

# Show version information
./apidiag --version
```

## Web Interface

Once the server is running, open your browser and navigate to:

```url
http://localhost:8080
```

### Interface Features

- **Interactive Diagram**: Zoom, pan, and explore the call graph
- **Filtering Controls**: Filter by various criteria:
  - Package names
  - Function names
  - File paths
  - Receiver types
  - Function signatures
  - Generic types
  - Scope (exported/unexported)
- **Pagination**: Navigate through large datasets
- **Export Options**: Download diagrams in various formats
- **Real-time Stats**: View analysis statistics and performance metrics

## API Endpoints

The server provides a RESTful API for programmatic access:

### Diagram Data

```bash
# Get complete diagram data
GET /api/diagram

# Get paginated diagram data
GET /api/diagram/page?page=1&size=100&depth=3

# Get diagram statistics
GET /api/diagram/stats

# Refresh metadata
POST /api/diagram/refresh

# Export diagram
GET /api/diagram/export?format=json

# Check server health
GET /health
```

### Query Parameters

The paginated endpoint supports advanced filtering:

- `page`: Page number (default: 1)
- `size`: Page size (default: 100, max: 2000)
- `depth`: Maximum call graph depth (default: 2)
- `package`: Filter by package names (comma-separated)
- `function`: Filter by function names (comma-separated)
- `file`: Filter by file paths (comma-separated)
- `receiver`: Filter by receiver types (comma-separated)
- `signature`: Filter by function signatures (comma-separated)
- `generic`: Filter by generic types (comma-separated)
- `scope`: Filter by scope (exported, unexported, all)

### Example API Calls

```bash
# Get first page with 50 items
curl "http://localhost:8080/api/diagram/page?page=1&size=50"

# Filter by specific package
curl "http://localhost:8080/api/diagram/page?package=github.com/myorg/mypackage"

# Filter by function name
curl "http://localhost:8080/api/diagram/page?function=GetUser,CreateUser"

# Export as JSON
curl "http://localhost:8080/api/diagram/export?format=json" > diagram.json
```

## Configuration

The diagram server uses the same configuration system as the main APISpec tool. You can provide a custom configuration file:

```bash
# Use custom config (if supported in future versions)
./diagram-server --config my-config.yaml
```

## Performance Considerations

For large codebases, consider these optimizations:

- **Adjust page size**: Use smaller page sizes for better performance
- **Limit depth**: Reduce max-depth for faster analysis
- **Enable caching**: The server caches results for 5 minutes by default
- **Exclude tests/mocks**: Use `--auto-exclude-tests` and `--auto-exclude-mocks`

## Versioning

apidiag includes comprehensive versioning information that can be displayed using the `--version` or `-V` flags:

```bash
# Show version information
./apidiag --version
```

## Troubleshooting

### Common Issues

1. **Server won't start**: Check if the port is already in use
2. **Empty diagram**: Ensure you're running in a directory with Go source files
3. **Slow performance**: Try reducing page size or max depth
4. **Memory issues**: For very large projects, consider excluding test files

### Debug Mode

Enable verbose logging to see detailed information:

```bash
./apidiag --verbose
```

### Health Check

Check if the server is running properly:

```bash
curl http://localhost:8080/health
```

## Contributing

To contribute to the diagram server:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

Apache License 2.0 - See the main project [LICENSE](../../LICENSE) for details.
