APP_NAME = swagen
VERSION = 0.0.1
COMMIT = $(shell git rev-parse --short HEAD)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION = $(shell go version | awk '{print $$3}')

LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: help build test clean coverage update-badge

# Default target
help:
	@echo "Available targets:"
	@echo "  build         - Build the swagen binary"
	@echo "  test          - Run all tests"
	@echo "  coverage      - Run tests with coverage report"
	@echo "  update-badge  - Update coverage badge in README.md"
	@echo "  clean         - Clean build artifacts"
	@echo "  help          - Show this help message"

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

run: build
	./$(APP_NAME) --version

version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(GO_VERSION)"
# Install dependencies
deps:
	go mod download
	go mod tidy
