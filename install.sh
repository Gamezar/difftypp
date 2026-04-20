#!/usr/bin/env bash

set -euo pipefail

REPO="Gamezar/difftypp"
BINARY_NAME="diffty"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *)
      printf 'Unsupported operating system: %s\n' "$(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)
      printf 'Unsupported architecture: %s\n' "$(uname -m)" >&2
      exit 1
      ;;
  esac
}

extract_tag() {
  awk -F '"' '/"tag_name"/ { print $4; exit }'
}

need_cmd curl
need_cmd mktemp
need_cmd chmod
need_cmd mv
need_cmd mkdir
need_cmd awk

OS="$(detect_os)"
ARCH="$(detect_arch)"
ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

printf 'Fetching latest release for %s...\n' "$ASSET_NAME"
RELEASE_JSON="$(curl -fsSL "$LATEST_URL")"
VERSION="$(printf '%s' "$RELEASE_JSON" | extract_tag)"

if [ -z "$VERSION" ]; then
  printf 'Failed to determine latest release version\n' >&2
  exit 1
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"
TMP_FILE="$(mktemp)"

cleanup() {
  rm -f "$TMP_FILE"
}
trap cleanup EXIT

printf 'Downloading %s...\n' "$DOWNLOAD_URL"
curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"

mkdir -p "$INSTALL_DIR"
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "$INSTALL_DIR/$BINARY_NAME"

printf 'Installed %s %s to %s\n' "$BINARY_NAME" "$VERSION" "$INSTALL_DIR/$BINARY_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf 'Add %s to your PATH if needed:\n' "$INSTALL_DIR"
    printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    ;;
esac

printf 'Run `%s --version` to verify installation.\n' "$BINARY_NAME"
printf 'Run `%s --update` anytime to upgrade to the latest release.\n' "$BINARY_NAME"
