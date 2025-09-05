#!/bin/bash

# APISpec Installation Script
# This script provides multiple installation options for apispec

set -e

APP_NAME="apispec"
VERSION="0.0.1"
REPO_URL="https://github.com/ehabterra/apispec"

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
    echo -e "${BLUE}  APISpec Installation Script${NC}"
    echo -e "${BLUE}================================${NC}"
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check Go installation
check_go() {
    if ! command_exists go; then
        print_error "Go is not installed. Please install Go first:"
        echo "  Visit: https://golang.org/doc/install"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}')
    print_status "Found Go version: $GO_VERSION"
}

# Function to install using go install
install_go_install() {
    print_status "Installing using 'go install'..."
    
    if command_exists apispec; then
        print_warning "apispec is already installed. Updating..."
        go install github.com/ehabterra/apispec/cmd/apispec@latest
    else
        go install github.com/ehabterra/apispec/cmd/apispec@latest
    fi
    
    # Check if installation was successful
    if command_exists apispec; then
        print_status "apispec installed successfully using go install!"
        apispec --version
    else
        print_error "Installation failed. Please check your Go environment."
        exit 1
    fi
}

# Function to install from source
install_from_source() {
    print_status "Installing from source..."
    
    # Create temporary directory
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    
    # Clone repository
    print_status "Cloning repository..."
    git clone "$REPO_URL" .
    
    # Build and install
    print_status "Building apispec..."
    make build
    
    # Install to system
    if [ "$1" = "system" ]; then
        print_status "Installing to /usr/local/bin (requires sudo)..."
        sudo cp apispec /usr/local/bin/
        print_status "apispec installed to /usr/local/bin successfully!"
    else
        print_status "Installing to ~/go/bin..."
        mkdir -p ~/go/bin
        cp apispec ~/go/bin/
        print_status "apispec installed to ~/go/bin successfully!"
        print_warning "Make sure ~/go/bin is in your PATH"
        echo "Add this to your shell profile: export PATH=\$HOME/go/bin:\$PATH"
    fi
    
    # Cleanup
    cd - > /dev/null
    rm -rf "$TEMP_DIR"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTION]"
    echo ""
    echo "Options:"
    echo "  go-install     Install using 'go install' (recommended)"
    echo "  source-local   Install from source to ~/go/bin"
    echo "  source-system  Install from source to /usr/local/bin (requires sudo)"
    echo "  help           Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 go-install      # Install using go install"
    echo "  $0 source-local    # Build and install to user directory"
    echo "  $0 source-system   # Build and install to system directory"
}

# Main script
main() {
    print_header
    
    # Check Go installation
    check_go
    
    # Parse arguments
    case "${1:-go-install}" in
        "go-install")
            install_go_install
            ;;
        "source-local")
            install_from_source "local"
            ;;
        "source-system")
            install_from_source "system"
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
    
    print_status "Installation completed successfully!"
    echo ""
    echo "You can now use apispec:"
    echo "  apispec --help          # Show help"
    echo "  apispec --version       # Show version"
    echo "  apispec <directory>     # Generate OpenAPI spec"
}

# Run main function
main "$@"
