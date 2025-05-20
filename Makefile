# ================================
# Variables
# ================================
GO             := go
GOFLAGS        := -mod=readonly
LDFLAGS        := -ldflags "-X main.version=$(VERSION)"
BUILD_DIR      := ./bin

# Main binaries (full paths)
SERVER_SRC     := cmd/kloud-kraken/main.go
CLIENT_SRC     := service/client.go
SERVER_BINARY  := kloud-kraken-server
CLIENT_BINARY  := kloud-kraken-client

VERSION        := $(shell git describe --tags --always --dirty)

# Cross-compilation targets
GOOS_LINUX     := linux
GOARCH_AMD64   := amd64
GOARCH_ARM64   := arm64

# ================================
# Phony targets
# ================================
.PHONY: all build test vet lint clean cross build-linux-amd64 \
		build-linux-arm64 run-server run-client install rebuild

# Default target
all: build

# Ensure build directory exists
$(BUILD_DIR):
	mkdir -p $@

# ================================
# Build (both server and client)
# ================================
build: | $(BUILD_DIR)
	@echo "Building all binaries [version: $(VERSION)]..."
	# Server
	$(GO) build $(GOFLAGS) $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(SERVER_BINARY) \
	  $(SERVER_SRC)
	# Client
	$(GO) build $(GOFLAGS) $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(CLIENT_BINARY) \
	  $(CLIENT_SRC)
	@echo "All builds completed."

# Cross-compile both binaries for Linux/amd64
.PHONY: build-linux-amd64
build-linux-amd64: | $(BUILD_DIR)
	@echo "Cross-compiling for Linux/amd64..."
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) \
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-amd64 $(SERVER_SRC)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) \
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY)-linux-amd64 $(CLIENT_SRC)
	@echo "Linux/amd64 cross-compiles completed."

# Cross-compile both binaries for Linux/arm64
.PHONY: build-linux-arm64
build-linux-arm64: | $(BUILD_DIR)
	@echo "Cross-compiling for Linux/arm64..."
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) \
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-arm64 $(SERVER_SRC)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) \
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY)-linux-arm64 $(CLIENT_SRC)
	@echo "Linux/arm64 cross-compiles completed."

# Alias to build all cross-compiled binaries
.PHONY: cross
cross: build-linux-amd64 build-linux-arm64
	@echo "All cross-compiles completed."

# ================================
# Quality checks
# ================================
.PHONY: test vet lint

test:
	@echo "Running tests with race detector and coverage..."
	$(GO) test -race -timeout=30s -cover ./...
	@echo "Tests completed."

vet:
	@echo "Running go vet..."
	$(GO) vet ./...
	@echo "go vet completed."

lint:
	@echo "Running golangci-lint..."
	golangci-lint run
	@echo "Lint completed."

# ================================
# Clean
# ================================
.PHONY: clean

clean:
	@echo "Cleaning build artifacts in $(BUILD_DIR)..."
	rm -rf $(BUILD_DIR)/*
	@echo "Clean completed."

# ================================
# Run & Install
# ================================
.PHONY: run-server run-client install

run-server:
	@echo "Running server..."
	$(GO) run $(SERVER_SRC)

run-client:
	@echo "Running client..."
	$(GO) run $(CLIENT_SRC)

install:
	@echo "Installing server and client..."
	$(GO) install $(SERVER_SRC)
	$(GO) install $(CLIENT_SRC)
	@echo "Installation completed."

# ================================
# Convenience
# ================================
.PHONY: rebuild

rebuild: clean build
	@echo "Rebuild completed."
