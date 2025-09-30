#!/bin/bash

# APISpec API Diagram Server Runner (apidiag)
# This script builds and runs the API diagram server for real-time pagination

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
PROJECT_DIR="."
PORT=8080
HOST="localhost"
PAGE_SIZE=100
MAX_DEPTH=3
VERBOSE=false

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Function to show usage
show_usage() {
    echo "APISpec API Diagram Server Runner (apidiag)"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -d, --dir DIR        Project directory to analyze (default: .)"
    echo "  -p, --port PORT      Server port (default: 8080)"
    echo "  -h, --host HOST      Server host (default: localhost)"
    echo "  -s, --page-size SIZE Page size for pagination (default: 100)"
    echo "  -m, --max-depth DEPTH Maximum call graph depth (default: 3)"
    echo "  -v, --verbose        Enable verbose logging"
    echo "  --help               Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --dir ./myproject --port 8080"
    echo "  $0 --dir ./myproject --page-size 50 --max-depth 2"
    echo "  $0 --dir ./myproject --port 3000 --verbose"
    echo ""
    echo "After starting the server:"
    echo "  1. Open http://localhost:8080 in your browser"
    echo "  2. Use the web interface to explore the call graph"
    echo "  3. Or use the API directly: curl http://localhost:8080/api/diagram/page?page=1&size=100"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dir)
            PROJECT_DIR="$2"
            shift 2
            ;;
        -p|--port)
            PORT="$2"
            shift 2
            ;;
        -h|--host)
            HOST="$2"
            shift 2
            ;;
        -s|--page-size)
            PAGE_SIZE="$2"
            shift 2
            ;;
        -m|--max-depth)
            MAX_DEPTH="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        --help)
            show_usage
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Validate inputs
if [[ ! -d "$PROJECT_DIR" ]]; then
    print_error "Project directory does not exist: $PROJECT_DIR"
    exit 1
fi

if ! [[ "$PORT" =~ ^[0-9]+$ ]] || [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
    print_error "Invalid port number: $PORT"
    exit 1
fi

if ! [[ "$PAGE_SIZE" =~ ^[0-9]+$ ]] || [ "$PAGE_SIZE" -lt 10 ] || [ "$PAGE_SIZE" -gt 1000 ]; then
    print_error "Invalid page size: $PAGE_SIZE (must be between 10 and 1000)"
    exit 1
fi

if ! [[ "$MAX_DEPTH" =~ ^[0-9]+$ ]] || [ "$MAX_DEPTH" -lt 1 ] || [ "$MAX_DEPTH" -gt 10 ]; then
    print_error "Invalid max depth: $MAX_DEPTH (must be between 1 and 10)"
    exit 1
fi

print_info "APISpec API Diagram Server Setup"
echo "================================"
print_info "Project directory: $PROJECT_DIR"
print_info "Server address: $HOST:$PORT"
print_info "Page size: $PAGE_SIZE nodes"
print_info "Max depth: $MAX_DEPTH levels"
print_info "Verbose logging: $VERBOSE"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_error "Go is not installed or not in PATH"
    exit 1
fi

print_success "Go is installed: $(go version)"

# Build the server
print_info "Building API diagram server..."
if go build -o apidiag ./cmd/apidiag; then
    print_success "Server built successfully"
else
    print_error "Failed to build server"
    exit 1
fi

# Create server-based HTML file
print_info "Generating server-based HTML client..."
if go run ./cmd/apispec --dir "$PROJECT_DIR" --diagram server-client.html --paginated-diagram; then
    print_success "Server-based HTML client generated: server-client.html"
else
    print_warning "Failed to generate server-based HTML client (continuing anyway)"
fi

# Start the server
print_info "Starting API diagram server..."
echo ""
print_success "🚀 Server starting on http://$HOST:$PORT"
print_info "📊 Analyzing project: $PROJECT_DIR"
print_info "⚙️  Page size: $PAGE_SIZE, Max depth: $MAX_DEPTH"
echo ""
print_info "Available endpoints:"
print_info "  • http://$HOST:$PORT/ - Web interface"
print_info "  • http://$HOST:$PORT/api/diagram/page - Paginated API"
print_info "  • http://$HOST:$PORT/api/diagram/stats - Statistics"
print_info "  • http://$HOST:$PORT/health - Health check"
echo ""
print_info "Press Ctrl+C to stop the server"
echo ""

# Build command arguments
SERVER_ARGS=(
    --dir "$PROJECT_DIR"
    --port "$PORT"
    --host "$HOST"
    --page-size "$PAGE_SIZE"
    --max-depth "$MAX_DEPTH"
)

if [ "$VERBOSE" = true ]; then
    SERVER_ARGS+=(--verbose)
fi

# Run the server
./apidiag "${SERVER_ARGS[@]}"
