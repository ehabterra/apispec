#!/bin/bash

# Create Release Script
# This script helps create a new release by creating a git tag and pushing it

set -e

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
    echo -e "${BLUE}  Swagen Release Creator${NC}"
    echo -e "${BLUE}================================${NC}"
}

# Function to check if we're in a git repository
check_git() {
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        print_error "Not in a git repository. Please run this from the project root."
        exit 1
    fi
}

# Function to check if there are uncommitted changes
check_clean_working_dir() {
    if ! git diff-index --quiet HEAD --; then
        print_warning "You have uncommitted changes in your working directory."
        print_warning "Please commit or stash them before creating a release."
        exit 1
    fi
}

# Function to check if the tag already exists
check_tag_exists() {
    local version=$1
    if git tag -l | grep -q "^v$version$"; then
        print_error "Tag v$version already exists."
        exit 1
    fi
}

# Function to validate version format
validate_version() {
    local version=$1
    if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        print_error "Invalid version format. Use semantic versioning (e.g., 1.0.0)"
        exit 1
    fi
}

# Function to create and push tag
create_release() {
    local version=$1
    local message=${2:-"Release version $version"}
    
    print_status "Creating release v$version..."
    
    # Create annotated tag
    git tag -a "v$version" -m "$message"
    
    print_status "Tag v$version created locally."
    print_warning "To complete the release, push the tag:"
    echo "  git push origin v$version"
    echo ""
    print_status "This will trigger the GitHub Actions release workflow."
}

# Function to show usage
show_usage() {
    echo "Usage: $0 <version> [message]"
    echo ""
    echo "Arguments:"
    echo "  version       Semantic version (e.g., 1.0.0)"
    echo "  message       Optional commit message (default: 'Release version X.Y.Z')"
    echo ""
    echo "Examples:"
    echo "  $0 1.0.0                           # Create release v1.0.0"
    echo "  $0 2.1.0 \"Major feature release\"  # Create release with custom message"
    echo ""
    echo "Note: After creating the tag, push it with: git push origin v<version>"
}

# Main script
main() {
    print_header
    
    # Check arguments
    if [ $# -eq 0 ] || [ "$1" = "-h" ] || [ "$1" = "--help" ] || [ "$1" = "help" ]; then
        show_usage
        exit 0
    fi
    
    local version=$1
    local message=${2:-"Release version $version"}
    
    # Validate version
    validate_version "$version"
    
    # Check git repository
    check_git
    
    # Check working directory is clean
    check_clean_working_dir
    
    # Check if tag already exists
    check_tag_exists "$version"
    
    # Create release
    create_release "$version" "$message"
}

# Run main function
main "$@"
