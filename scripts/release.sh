#!/bin/bash

# Swagen Release Script
# This script builds swagen for multiple platforms and creates a release package

set -e

APP_NAME="swagen"
VERSION="0.0.1"
COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION=$(go version | awk '{print $3}')

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}================================${NC}"
    echo -e "${BLUE}  Swagen Release Script${NC}"
    echo -e "${BLUE}================================${NC}"
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check Go installation
check_go() {
    if ! command_exists go; then
        print_error "Go is not installed. Please install Go first."
        exit 1
    fi
    
    print_status "Found Go version: $GO_VERSION"
}

# Function to build for a specific platform
build_for_platform() {
    local GOOS=$1
    local GOARCH=$2
    local EXTENSION=$3
    
    print_status "Building for $GOOS/$GOARCH..."
    
    # Set environment variables
    export GOOS=$GOOS
    export GOARCH=$GOARCH
    export CGO_ENABLED=0
    
    # Build flags
    LDFLAGS="-X 'main.Version=$VERSION' -X 'main.Commit=$COMMIT' -X 'main.BuildDate=$BUILD_DATE' -X 'main.GoVersion=$GO_VERSION'"
    
    # Build
    go build -ldflags "$LDFLAGS" -o "dist/${APP_NAME}-${GOOS}-${GOARCH}${EXTENSION}" ./cmd/swagen
    
    print_status "Built: dist/${APP_NAME}-${GOOS}-${GOARCH}${EXTENSION}"
}

# Function to create release package
create_release_package() {
    print_status "Creating release package..."
    
    # Create dist directory if it doesn't exist
    mkdir -p dist
    
    # Build for multiple platforms
    build_for_platform "linux" "amd64" ""
    build_for_platform "linux" "arm64" ""
    build_for_platform "darwin" "amd64" ""
    build_for_platform "darwin" "arm64" ""
    build_for_platform "windows" "amd64" ".exe"
    build_for_platform "windows" "arm64" ".exe"
    
    # Create checksums
    print_status "Creating checksums..."
    cd dist
    for file in *; do
        if [[ -f "$file" ]]; then
            shasum -a 256 "$file" > "$file.sha256"
        fi
    done
    cd ..
    
    # Create release archive
    RELEASE_NAME="${APP_NAME}-${VERSION}"
    print_status "Creating release archive: $RELEASE_NAME.tar.gz"
    
    cd dist
    tar -czf "../$RELEASE_NAME.tar.gz" *
    cd ..
    
    print_status "Release package created: $RELEASE_NAME.tar.gz"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTION]"
    echo ""
    echo "Options:"
    echo "  build         Build for all supported platforms"
    echo "  clean         Clean build artifacts"
    echo "  help          Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 build      # Build for all platforms"
    echo "  $0 clean      # Clean build artifacts"
}

# Function to clean build artifacts
clean_build() {
    print_status "Cleaning build artifacts..."
    rm -rf dist/
    rm -f "${APP_NAME}-${VERSION}.tar.gz"
    print_status "Cleanup completed"
}

# Main script
main() {
    print_header
    print_status "Version: $VERSION"
    print_status "Commit: $COMMIT"
    print_status "Build Date: $BUILD_DATE"
    echo ""
    
    # Check Go installation
    check_go
    
    # Parse arguments
    case "${1:-build}" in
        "build")
            create_release_package
            ;;
        "clean")
            clean_build
            ;;
        "help"|"-h"|"--help")
            show_usage
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
    
    print_status "Release process completed successfully!"
}

# Run main function
main "$@"
