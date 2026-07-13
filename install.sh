#!/bin/bash
set -e

REPO_URL="https://github.com/Nomadcxx/plex2jellyfin.git"
INSTALL_DIR="/tmp/plex2jellyfin-install-$$"

echo "Plex2Jellyfin Installer"
echo "===================="
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "Error: This installer must be run as root"
    echo "Please run: sudo $0"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed"
    echo "Please install Go 1.25 or later from https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.25"

if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "Error: Go version $GO_VERSION is too old"
    echo "Please install Go $REQUIRED_VERSION or later"
    exit 1
fi

if command -v git &> /dev/null; then
    echo "Cloning repository..."
    git clone --depth 1 "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
else
    echo "Warning: git not found. Assuming we're already in the repository."
    INSTALL_DIR="$(pwd)"
fi

echo "Building installer..."
go build -o installer ./cmd/installer

echo ""
echo "Starting interactive installer..."
echo ""

./installer

EXIT_CODE=$?

if [ -d "/tmp/plex2jellyfin-install-$$" ]; then
    rm -rf "$INSTALL_DIR"
fi

exit $EXIT_CODE
