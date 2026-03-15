#!/bin/sh
set -e

REPO="kint-pro/kint-vault-cli"
BINARY="kint-vault"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

VERSION=$(curl -sfL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"v//;s/".*//')
if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version" && exit 1
fi

URL="https://github.com/$REPO/releases/download/v${VERSION}/kint-vault-cli_${VERSION}_${OS}_${ARCH}.tar.gz"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading kint-vault v${VERSION} (${OS}/${ARCH})..."
curl -sfL "$URL" | tar -xz -C "$TMPDIR"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

echo "Installed $BINARY v${VERSION} to $INSTALL_DIR/$BINARY"
