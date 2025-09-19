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
TEARDOWN_SRC   := teardown/teardown.go

SERVER_BINARY  := kloud-kraken-server
CLIENT_BINARY  := kloud-kraken-client
TEARDOWN_BINARY:= kloud-kraken-teardown

VERSION        := $(shell git describe --tags --always --dirty)

# Cross-compilation targets
GOOS_LINUX     := linux
GOARCH_AMD64   := amd64
GOARCH_ARM64   := arm64

# Host detection
HOST_GOARCH := $(shell go env GOHOSTARCH)
HOST_GOOS   := $(shell go env GOHOSTOS)

# ================================
# Phony targets
# ================================
.PHONY: all build test vet lint clean cross build-linux-amd64 \
        build-linux-arm64 run-server run-client run-teardown build-host \
        build-host-teardown install rebuild

# Default target
all: build

# Ensure build directory exists
$(BUILD_DIR):
    mkdir -p $@

# ================================
# Build (server, client, teardown)
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
    # Teardown
    $(GO) build $(GOFLAGS) $(LDFLAGS) \
      -o $(BUILD_DIR)/$(TEARDOWN_BINARY) \
      $(TEARDOWN_SRC)
    @echo "All builds completed."

# Build for the current host's OS/ARCH (binaries suffixed with -os-arch)
.PHONY: build-host build-host-teardown
build-host: | $(BUILD_DIR)
    @echo "Building for host: $(HOST_GOOS)/$(HOST_GOARCH) [version: $(VERSION)]..."
    GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-$(HOST_GOOS)-$(HOST_GOARCH) $(SERVER_SRC)
    GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY)-$(HOST_GOOS)-$(HOST_GOARCH) $(CLIENT_SRC)
    GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(TEARDOWN_BINARY)-$(HOST_GOOS)-$(HOST_GOARCH) $(TEARDOWN_SRC)
    @echo "Host builds completed."

build-host-teardown: | $(BUILD_DIR)
    @echo "Building teardown for host: $(HOST_GOOS)/$(HOST_GOARCH)..."
    GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(TEARDOWN_BINARY)-$(HOST_GOOS)-$(HOST_GOARCH) $(TEARDOWN_SRC)
    @echo "Teardown host build completed."

# Cross-compile both binaries for Linux/amd64
.PHONY: build-linux-amd64
build-linux-amd64: | $(BUILD_DIR)
    @echo "Cross-compiling for Linux/amd64..."
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-amd64 $(SERVER_SRC)
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY)-linux-amd64 $(CLIENT_SRC)
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_AMD64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(TEARDOWN_BINARY)-linux-amd64 $(TEARDOWN_SRC)
    @echo "Linux/amd64 cross-compiles completed."

# Cross-compile both binaries for Linux/arm64
.PHONY: build-linux-arm64
build-linux-arm64: | $(BUILD_DIR)
    @echo "Cross-compiling for Linux/arm64..."
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-arm64 $(SERVER_SRC)
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY)-linux-arm64 $(CLIENT_SRC)
    GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH_ARM64) \
    $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(TEARDOWN_BINARY)-linux-arm64 $(TEARDOWN_SRC)
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
.PHONY: run-server run-client run-teardown install

run-server:
    @echo "Running server..."
    $(GO) run $(SERVER_SRC)

run-client:
    @echo "Running client..."
    $(GO) run $(CLIENT_SRC)

run-teardown:
    @echo "Running teardown..."
    $(GO) run $(TEARDOWN_SRC)

install:
    @echo "Installing server, client and teardown..."
    $(GO) install $(SERVER_SRC)
    $(GO) install $(CLIENT_SRC)
    $(GO) install $(TEARDOWN_SRC)
    @echo "Installation completed."

# ================================
# Convenience
# ================================
.PHONY: rebuild

rebuild: clean build
    @echo "Rebuild completed."
