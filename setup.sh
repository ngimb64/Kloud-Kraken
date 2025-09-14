#!/bin/bash
set -euo pipefail
# Set so APT packages install in noninteractive mode
export DEBIAN_FRONTEND=noninteractive

# Function to handle errors with custom messages and exit codes
error_exit() {
    local message="$1"
    local code="${2:-1}"  # Default exit code is 1 if not provided
    echo "[*] Error:  $message"
    exit "$code"
}


echo "===== Build script started ====="

# Ensure the system is updated
(apt-get update && apt-get upgrade -y) || error_exit "APT update/upgrade failed" 1

# Detect original users home
if [[ -n "$SUDO_USER" ]]; then
    USER_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
else
    USER_HOME="$HOME"
fi

# Detect user shell rc file
shell_name="$(basename "${SHELL:-}")"
case "$shell_name" in
    zsh) rcfile="$USER_HOME/.zshrc" ;;
    bash) rcfile="$USER_HOME/.bashrc" ;;
    *) rcfile="$USER_HOME/.profile" ;;
esac

echo "[->] Using shell rc file:  $rcfile"

echo "[->] Checking duplicut binary permissions"
duplicut_file="duplicut/duplicut"
# If the duplicut binary was not found
if [[ ! -f "$duplicut_file" ]]; then
    error_exit "No duplicut binary found in ./duplicut, try re-downloading project" 2
fi

# If executable permissions are not set, set them
if [[ ! -x "$duplicut_file" ]]; then
    echo "[+] duplicut is not executable .. setting +x"
    chmod +x "$duplicut_file" && ls -la "$duplicut_file"
fi

# If Go is not installed, install it
if ! command -v go >/dev/null 2>&1; then
    echo "[-] Go not found .. installing golang via apt"
    apt-get install -y golang || error_exit "Failed to install golang" 3
fi

echo "[->] Go version: $(go version)"

echo "[->] Ensuring Go environment variables are present in $rcfile"
# If the rcfile path is missing .. create it
if [[ ! -f "$rcfile" ]]; then
    echo "[-] RC file $rcfile not found, creating it."
    touch "$rcfile"
fi

# Go environment lines
go_env_lines=(
    'export GOROOT=/usr/lib/go'
    "export GOPATH=$USER_HOME/go"
    "export PATH=\$GOPATH/bin:\$GOROOT/bin:\$PATH"
)

# Iterate through each line in go env lines list
for line in "${go_env_lines[@]}"; do
    # If the line does not exist in the file, add it
    if ! grep -Fqx "$line" "$rcfile"; then
        printf '%s\n' "$line" >> "$rcfile"
    fi
done

# Reload rc file for this script
echo "[->] Sourcing $rcfile to update environment for this script"
eval "$(sed -n '/esac/,$p' "$rcfile" | sed '1d')"

# Ensure GOPATH directories exist
mkdir -p "$USER_HOME/go"/{bin,src,pkg}
echo "GOPATH set to: ${GOPATH:-$USER_HOME/go}"

echo "[->] GOPATH: $GOPATH"
echo "[->] GOROOT: $GOROOT"

# Ensure project dependencies are installed and resolved
if [[ ! -f "go.mod" ]]; then
    error_exit "Required go.mod missing from project, try redownloading project" 4
fi

echo "[->] Ensuring dependencies are installed & resolved:  go get ./... && go mod tidy -e"
(go get ./... && go mod tidy -e ) || error_exit "Failed to resolve Go dependencies" 5

# Executing test cases
echo "[->] Running unit tests:  go test ./..."
go test ./... || error_exit "Test case failue" 6

# Building binaries with make
echo "[->] Building binaries:  make all"
if ! command -v make >/dev/null 2>&1; then
    echo "[-] make not found .. installing build-essential"
    apt-get install -y build-essential || error_exit "Failed to install build-essential" 7
fi

make all || error_exit "Make command failed" 8

echo "===== Build script finished successfully ====="
