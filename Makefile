APP_NAME = apispec
VERSION ?= 0.0.1
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION ?= $(shell go version | awk '{print $$3}')

LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: help build test clean coverage lint fmt update-badge metrics-view metrics-generate

# Default target
help:
	@echo "Available targets:"
	@echo "  build         - Build the apispec binary"
	@echo "  build-apidiag - Build the apidiag binary"
	@echo "  install       - Install apispec to /usr/local/bin (requires sudo)"
	@echo "  install-local - Install apispec to ~/go/bin (no sudo required)"
	@echo "  uninstall     - Remove apispec from /usr/local/bin"
	@echo "  uninstall-local - Remove apispec from ~/go/bin"
	@echo "  release       - Build for multiple platforms and create release package"
	@echo "  create-tag    - Create a new release tag (e.g., make create-tag VERSION=1.0.0)"
	@echo "  tags          - Show current git tags"
	@echo "  test          - Run all tests"
	@echo "  coverage      - Run tests with coverage report"
	@echo "  lint          - Run linting checks (golangci-lint, go vet, go fmt)"
	@echo "  fmt           - Format Go code"
	@echo "  update-badge  - Update coverage badge in README.md"
	@echo "  clean         - Clean build artifacts"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Metrics Visualization:"
	@echo "  metrics-view     - Interactive metrics viewer (opens menu)"
	@echo "  metrics-generate - Generate metrics with profiling (selectable directory)"
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
	@echo "  make lint                     # Run all linting checks"
	@echo "  make fmt                      # Format code"
	@echo "  make release                  # Create release package"
	@echo "  make create-tag VERSION=1.0.0 # Create release tag v1.0.0"

# Build the apispec binary
build:
	@echo "Building $(APP_NAME) version $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) ./cmd/apispec

# Build the apidiag binary
build-apidiag:
	@echo "Building apidiag version $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o apidiag ./cmd/apidiag

# Run all tests
test:
	go test ./...

# Run tests with coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linting checks
lint:
	@echo "Running golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.4.0; \
	fi
	golangci-lint run --timeout=5m
	@echo "Running go vet..."
	go vet ./...
	@echo "Checking code formatting..."
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "The following files are not formatted:"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	@echo "All linting checks passed!"

# Format Go code
fmt:
	@echo "Formatting Go code..."
	gofmt -s -w .
	@echo "Code formatted!"

# Update coverage badge in README.md
update-badge:
	./scripts/update-coverage-badge.sh

# Clean build artifacts
clean:
	rm -f $(APP_NAME) apidiag coverage.out coverage.html
	go clean -cache

# Install apispec to system (requires sudo)
install: build
	@echo "Installing $(APP_NAME) to /usr/local/bin/..."
	sudo cp $(APP_NAME) /usr/local/bin/
	@echo "$(APP_NAME) installed successfully!"
	@echo "You can now run 'apispec --help' from anywhere"

# Install to user's local bin directory (no sudo required)
install-local: build
	@echo "Installing $(APP_NAME) to ~/go/bin/..."
	mkdir -p ~/go/bin
	cp $(APP_NAME) ~/go/bin/
	@echo "$(APP_NAME) installed successfully!"
	@echo "Make sure ~/go/bin is in your PATH"
	@echo "Add this to your shell profile: export PATH=\$$HOME/go/bin:\$$PATH"

# Uninstall apispec from system
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

# Metrics visualization commands
metrics-view:
	@echo "Opening interactive metrics viewer..."
	@./scripts/view_metrics.sh

metrics-generate:
	@echo "Generating metrics with profiling enabled..."
	@echo "Available directories in testdata/:"
	@ls -1d */ 2>/dev/null | sed 's|/||' | head -10 || echo "  (no subdirectories found)"
	@echo ""
	@read -p "Enter directory path (or press Enter for testdata/chi): " dir; \
	if [ -z "$$dir" ]; then dir="testdata/chi"; fi; \
	if [ ! -d "$$dir" ]; then \
		echo "Error: Directory '$$dir' not found"; \
		exit 1; \
	fi; \
	echo "Generating metrics for directory: $$dir"; \
	./apispec --dir "$$dir" --output "$$(basename "$$dir")-output.json" --custom-metrics --verbose --metrics-path "$$(basename "$$dir")-metrics.json"; \
	echo "Metrics generated in profiles/$$(basename "$$dir")-metrics.json"
