# Variables
GO = go
GOFLAGS =
BUILD_DIR = ./bin
MAIN_PACKAGE = cmd/kloud-kraken
BINARY_NAME = KloudKraken
VERSION = $(shell git describe --tags --always --dirty)

# Cross-compilation for multiple platforms
GOOS_LINUX = linux
GOARCH_AMD64 = amd64
GOARCH_ARM64 = arm64

# Default target
.PHONY: all
all: build

# Build the application
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Build completed."

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GO) test ./...
	@echo "Tests completed."

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)/*
	@echo "Clean completed."

# Build for Linux AMD64
.PHONY: build-linux-amd64
build-linux-amd64:
	@echo "Building for Linux AMD64..."
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	@echo "Build for Linux AMD64 completed."

# Build for Linux ARM64
.PHONY: build-linux-arm64
build-linux-arm64:
	@echo "Building for Linux ARM64..."
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	@echo "Build for Linux ARM64 completed."

# Build for MacOS
.PHONY: build-macos
build-macos:
	@echo "Building for MacOS..."
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-macos $(MAIN_PACKAGE)
	@echo "Build for MacOS completed."

# Build for Windows
.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows.exe $(MAIN_PACKAGE)
	@echo "Build for Windows completed."

# Build with version
.PHONY: build-version
build-version:
	@echo "Building with version $(VERSION)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION) $(MAIN_PACKAGE)
	@echo "Build completed with version $(VERSION)."

# Run the application
.PHONY: run
run:
	@echo "Running the application..."
	$(GO) run $(MAIN_PACKAGE)

# Install the Go application
.PHONY: install
install:
	@echo "Installing application..."
	$(GO) install $(MAIN_PACKAGE)
	@echo "Application installed."

# Clean and then build
.PHONY: rebuild
rebuild: clean build
