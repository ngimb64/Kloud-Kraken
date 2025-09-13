#!/bin/bash
set -euo pipefail
# Set so APT packages install in noninteractive mode
export DEBIAN_FRONTEND=noninteractive

echo "===== Build script started ====="

# Ensure the system is updated
apt-get update && apt-get upgrade -y

# Detect user shell rc file
shell_name="$(basename "${SHELL:-}")"
case "$shell_name" in
    zsh) rcfile="$HOME/.zshrc" ;;
    bash) rcfile="$HOME/.bashrc" ;;
    *) rcfile="$HOME/.profile" ;;
esac

echo "[->] Using shell rc file:  $rcfile"

echo "[->] Checking duplicut binary permissions"
duplicut_file="duplicut/duplicut"
# If the duplicut binary was not found
if [[ ! -f "$duplicut_file" ]]; then
    echo "[*] No duplicut binary found in ./duplicut, try re-downloading project"
    exit 1
fi

# If executable permissions are not set, set them
if [[ ! -x "$duplicut_file" ]]; then
    echo "[+] duplicut is not executable .. setting +x"
    chmod +x "$duplicut_file" && ls -la "$duplicut_file"
fi

# If Go is not installed, install it
if ! command -v go >/dev/null 2>&1; then
    echo "[-] Go not found .. installing golang via apt"
    apt-get install -y golang
fi

echo "[->] Go version: $(go version || echo 'go binary not found')"

echo "[->] Ensuring Go environment variables are present in $rcfile"
# If the rcfile path is missing .. create it
if [[ ! -f "$rcfile" ]]; then
    echo "[-] RC file $rcfile not found, creating it."
    touch "$rcfile"
fi

# Detect original users home
if [ -n "$SUDO_USER" ]; then
    USER_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
else
    USER_HOME="$HOME"
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
# shellcheck disable=SC1090
source "$rcfile"

# Ensure GOPATH directories exist
mkdir -p "$USER_HOME/go"/{bin,src,pkg}
echo "GOPATH set to: ${GOPATH:-$USER_HOME/go}"

# Ensure project dependencies are installed and resolved
if [[ ! -f "go.mod" ]]; then
    echo "[*] Required go.mod missing from project, try redownloading project"
    exit 2
fi

echo "[->] Ensuring dependencies are installed & resolved:  go get ./... && go mod tidy -e"
go get ./... && go mod tidy -e

# Executing test cases
echo "[->] Running unit tests:  go test ./..."
go test ./...

# Building binaries with make
echo "[->] Building binaries:  make all"
if ! command -v make >/dev/null 2>&1; then
    echo "[-] make not found .. installing build-essential"
    apt-get install -y build-essential
fi

make all

echo
echo "===== Build script finished successfully ====="
