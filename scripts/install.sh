#!/bin/bash

# APISpec Installation Script
# This script provides multiple installation options for apispec and apispecui.
#
# Which tools to install is chosen with --tool (default: both), so the same
# invocations that installed apispec keep working and now cover the UI too.

set -e

APP_NAME="apispec"
UI_NAME="apispecui"
# Tools to act on, set by --tool. Both by default: someone running the
# installer wants a working setup, and the UI is part of one.
TOOLS=("apispec" "apispecui")
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

    local tool
    for tool in "${TOOLS[@]}"; do
        if command_exists "$tool"; then
            print_warning "$tool is already installed. Updating..."
        fi
        go install "github.com/ehabterra/apispec/cmd/$tool@latest"

        # Check if installation was successful
        if command_exists "$tool"; then
            print_status "$tool installed successfully using go install!"
            "$tool" --version | head -n 1
        else
            print_error "Installation of $tool failed. Please check your Go environment."
            exit 1
        fi
    done
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
    
    # Build and install each selected tool
    local tool target
    for tool in "${TOOLS[@]}"; do
        print_status "Building $tool..."
        if [ "$tool" = "$UI_NAME" ]; then
            make build-ui
        else
            make build
        fi

        if [ "$1" = "system" ]; then
            target="/usr/local/bin"
            print_status "Installing $tool to $target (requires sudo)..."
            sudo cp "$tool" "$target/"
        else
            target="$HOME/go/bin"
            print_status "Installing $tool to $target..."
            mkdir -p "$target"
            cp "$tool" "$target/"
        fi
        print_status "$tool installed to $target successfully!"
    done

    if [ "$1" != "system" ]; then
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
    echo "Flags:"
    echo "  --tool NAME    Which tool to install: apispec, apispecui, or both"
    echo "                 (default: both)"
    echo ""
    echo "Examples:"
    echo "  $0 go-install                    # Install both tools with go install"
    echo "  $0 go-install --tool apispec     # CLI only"
    echo "  $0 go-install --tool apispecui   # Web UI only"
    echo "  $0 source-local    # Build and install to user directory"
    echo "  $0 source-system   # Build and install to system directory"
}

# Main script
main() {
    print_header

    # Check Go installation
    check_go

    # Pull --tool out of the arguments before the mode is read, so it can be
    # given on either side of it.
    local args=()
    while [ $# -gt 0 ]; do
        case "$1" in
            --tool)
                shift
                case "${1:-}" in
                    apispec)   TOOLS=("$APP_NAME") ;;
                    apispecui) TOOLS=("$UI_NAME") ;;
                    both|"")   TOOLS=("$APP_NAME" "$UI_NAME") ;;
                    *)
                        print_error "Unknown tool: $1 (expected apispec, apispecui or both)"
                        exit 1
                        ;;
                esac
                ;;
            *) args+=("$1") ;;
        esac
        shift
    done
    set -- "${args[@]}"

    print_status "Installing: ${TOOLS[*]}"

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
            # Return rather than fall through: the summary below claims an
            # installation happened, which printing help plainly did not.
            show_usage
            return 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
    
    print_status "Installation completed successfully!"
    echo ""
    local tool
    for tool in "${TOOLS[@]}"; do
        if [ "$tool" = "$UI_NAME" ]; then
            echo "You can now use apispecui:"
            echo "  apispecui --help          # Show help"
            echo "  apispecui --version       # Show version"
            echo "  apispecui -d <directory>  # Open the web UI on http://localhost:8088"
        else
            echo "You can now use apispec:"
            echo "  apispec --help          # Show help"
            echo "  apispec --version       # Show version"
            echo "  apispec <directory>     # Generate OpenAPI spec"
        fi
        echo ""
    done
}

# Run main function
main "$@"
