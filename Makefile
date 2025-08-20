APP_NAME = swagen
VERSION = 0.0.1
COMMIT = $(shell git rev-parse --short HEAD)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION = $(shell go version | awk '{print $$3}')

LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: build clean run version

build:
	@echo "Building $(APP_NAME) version $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) ./cmd

clean:
	rm -f $(APP_NAME)

run: build
	./$(APP_NAME) --version

version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(GO_VERSION)"
