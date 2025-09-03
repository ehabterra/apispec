APP_NAME = swagen
VERSION ?= 0.0.1
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION ?= $(shell go version | awk '{print $$3}')

LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: help build test clean coverage update-badge

# Default target
help:
	@echo "Available targets:"
	@echo "  build         - Build the swagen binary"
	@echo "  install       - Install swagen to /usr/local/bin (requires sudo)"
	@echo "  install-local - Install swagen to ~/go/bin (no sudo required)"
	@echo "  uninstall     - Remove swagen from /usr/local/bin"
	@echo "  uninstall-local - Remove swagen from ~/go/bin"
	@echo "  release       - Build for multiple platforms and create release package"
	@echo "  create-tag    - Create a new release tag (e.g., make create-tag VERSION=1.0.0)"
	@echo "  tags          - Show current git tags"
	@echo "  test          - Run all tests"
	@echo "  coverage      - Run tests with coverage report"
	@echo "  update-badge  - Update coverage badge in README.md"
	@echo "  clean         - Clean build artifacts"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Environment Variables:"
	@echo "  VERSION       - Override version (default: auto-detected)"
	@echo "  COMMIT        - Override commit hash (default: auto-detected)"
	@echo "  BUILD_DATE    - Override build date (default: auto-detected)"
	@echo "  GO_VERSION    - Override Go version (default: auto-detected)"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build with auto-detected values"
	@echo "  make VERSION=1.0.0 build     # Build with specific version"
	@echo "  make release                  # Create release package"
	@echo "  make create-tag VERSION=1.0.0 # Create release tag v1.0.0"

# Build the swagen binary
build:
	@echo "Building $(APP_NAME) version $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) ./cmd/swagen

# Run all tests
test:
	go test ./...

# Run tests with coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Update coverage badge in README.md
update-badge:
	./scripts/update-coverage-badge.sh

# Clean build artifacts
clean:
	rm -f $(APP_NAME) coverage.out coverage.html
	go clean -cache

# Install swagen to system (requires sudo)
install: build
	@echo "Installing $(APP_NAME) to /usr/local/bin/..."
	sudo cp $(APP_NAME) /usr/local/bin/
	@echo "$(APP_NAME) installed successfully!"
	@echo "You can now run 'swagen --help' from anywhere"

# Install to user's local bin directory (no sudo required)
install-local: build
	@echo "Installing $(APP_NAME) to ~/go/bin/..."
	mkdir -p ~/go/bin
	cp $(APP_NAME) ~/go/bin/
	@echo "$(APP_NAME) installed successfully!"
	@echo "Make sure ~/go/bin is in your PATH"
	@echo "Add this to your shell profile: export PATH=\$$HOME/go/bin:\$$PATH"

# Uninstall swagen from system
uninstall:
	@echo "Uninstalling $(APP_NAME) from /usr/local/bin/..."
	sudo rm -f /usr/local/bin/$(APP_NAME)
	@echo "$(APP_NAME) uninstalled successfully!"

# Uninstall from user's local bin directory
uninstall-local:
	@echo "Uninstalling $(APP_NAME) from ~/go/bin/..."
	rm -f ~/go/bin/$(APP_NAME)
	@echo "$(APP_NAME) uninstalled successfully!"

# Build for multiple platforms and create release package
release:
	@echo "Creating release package for $(APP_NAME) version $(VERSION)..."
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_DATE=$(BUILD_DATE) GO_VERSION=$(GO_VERSION) ./scripts/release.sh build

# Create a new release tag
create-tag:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Use: make create-tag VERSION=1.0.0"; \
		exit 1; \
	fi
	@echo "Creating release tag v$(VERSION)..."
	./scripts/create-release.sh $(VERSION)

run: build
	./$(APP_NAME) --version

version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(GO_VERSION)"

# Show current git tags
tags:
	@echo "Current git tags:"
	@git tag -l --sort=-version:refname | head -10
# Install dependencies
deps:
	go mod download
	go mod tidy
