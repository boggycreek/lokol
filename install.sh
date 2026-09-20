#!/usr/bin/env bash
# Copyright (c) 2026 Boggy Creek Software LLC
#
# lokol Installer
#
# Supports macOS (Darwin) and Linux (all standard distributions).
#
# Install via curl:
#   curl -fsSL https://raw.githubusercontent.com/boggycreek/lokol/main/install.sh | bash
#
# Or run from local clone:
#   ./install.sh
#
# Options:
#   --version <ver>   Specify a specific release version/tag (default: latest release)
#   --bin-dir <path>  Override target binary directory (default: ~/.local/bin)
#   --build-from-source Force building from source repository via git clone & go build
#   -h, --help        Show usage information.

set -euo pipefail

REPO_OWNER="boggycreek"
REPO_NAME="lokol"
REPO_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}.git"

# XDG Base Directory specification paths
XDG_BIN_HOME="${HOME}/.local/bin"
XDG_DATA_HOME="${XDG_DATA_HOME:-${HOME}/.local/share}/lokol"
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-${HOME}/.config}/lokol"

TARGET_BIN_DIR="${XDG_BIN_HOME}"
REQUESTED_VERSION="latest"
BUILD_FROM_SOURCE=false

print_usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --version <tag>      Specify release version to install (e.g. v0.1.0-alpha.1, or 'latest')
  --bin-dir <path>     Target directory for lokol executable (default: ~/.local/bin)
  --build-from-source  Force local git clone and Go compilation instead of prebuilt binary
  -h, --help           Show this help message
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      REQUESTED_VERSION="$2"
      shift 2
      ;;
    --bin-dir)
      TARGET_BIN_DIR="$2"
      shift 2
      ;;
    --build-from-source)
      BUILD_FROM_SOURCE=true
      shift
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      echo "Error: Unknown option: $1" >&2
      print_usage >&2
      exit 1
      ;;
  esac
done

echo "========================================"
echo "          lokol Installer               "
echo "========================================"
echo

# 1. Platform Detection
echo "[1/4] Detecting operating system and architecture..."
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"

case "${OS_RAW}" in
  Darwin)
    OS="darwin"
    OS_PRETTY="macOS"
    ;;
  Linux)
    OS="linux"
    OS_PRETTY="Linux"
    ;;
  *)
    echo "Error: Unsupported operating system: ${OS_RAW}" >&2
    exit 1
    ;;
esac

case "${ARCH_RAW}" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture: ${ARCH_RAW}" >&2
    exit 1
    ;;
esac

echo "  Platform: ${OS_PRETTY} (${OS}/${ARCH})"

# 2. Directory Preparation (XDG Compliance)
echo
echo "[2/4] Preparing XDG installation directories..."
mkdir -p "${TARGET_BIN_DIR}"
mkdir -p "${XDG_DATA_HOME}"
mkdir -p "${XDG_CONFIG_HOME}"

echo "  Binary Directory : ${TARGET_BIN_DIR}"
echo "  Data Directory   : ${XDG_DATA_HOME}"
echo "  Config Directory : ${XDG_CONFIG_HOME}"

# 3. Installation Execution
echo
echo "[3/4] Installing lokol binary..."

install_from_source() {
  echo "  Building lokol from source..."
  if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go compiler is required to build from source." >&2
    exit 1
  fi

  TMP_SRC="$(mktemp -d)"
  trap 'rm -rf "${TMP_SRC}"' EXIT

  echo "  Cloning ${REPO_URL} into temporary workspace..."
  git clone --depth 1 "${REPO_URL}" "${TMP_SRC}/lokol"

  (
    cd "${TMP_SRC}/lokol"
    make build
    cp -f "bin/lokol" "${TARGET_BIN_DIR}/lokol"
  )
}

CURRENT_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"

if [ "${BUILD_FROM_SOURCE}" = false ] && [ -n "${CURRENT_SCRIPT_DIR}" ] && [ -f "${CURRENT_SCRIPT_DIR}/go.mod" ] && [ -d "${CURRENT_SCRIPT_DIR}/cmd/lokol" ]; then
  # Running from inside existing local clone
  echo "  Detected existing local repository checkout at: ${CURRENT_SCRIPT_DIR}"
  if command -v go >/dev/null 2>&1; then
    echo "  Compiling static binary..."
    (cd "${CURRENT_SCRIPT_DIR}" && make build)
    cp -f "${CURRENT_SCRIPT_DIR}/bin/lokol" "${TARGET_BIN_DIR}/lokol"
  else
    echo "Error: Go compiler required when installing from local source checkout." >&2
    exit 1
  fi
elif [ "${BUILD_FROM_SOURCE}" = false ]; then
  # Attempt to fetch prebuilt static binary from GitHub Releases
  ARTIFACT_NAME="lokol-${OS}-${ARCH}"
  TARBALL_NAME="${ARTIFACT_NAME}.tar.gz"

  if [ "${REQUESTED_VERSION}" = "latest" ]; then
    RELEASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/latest/download/${TARBALL_NAME}"
  else
    RELEASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${REQUESTED_VERSION}/${TARBALL_NAME}"
  fi

  echo "  Attempting to download prebuilt static release: ${TARBALL_NAME}..."
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "${TMP_DIR}"' EXIT

  HTTP_STATUS="$(curl -sSL -w "%{http_code}" -o "${TMP_DIR}/${TARBALL_NAME}" "${RELEASE_URL}" || echo "000")"

  if [ "${HTTP_STATUS}" = "200" ] && [ -s "${TMP_DIR}/${TARBALL_NAME}" ]; then
    echo "  Extracting binary..."
    tar -xzf "${TMP_DIR}/${TARBALL_NAME}" -C "${TMP_DIR}"
    mv -f "${TMP_DIR}/${ARTIFACT_NAME}" "${TARGET_BIN_DIR}/lokol"
    chmod +x "${TARGET_BIN_DIR}/lokol"
    echo "  Successfully downloaded and installed static binary to ${TARGET_BIN_DIR}/lokol"
  else
    echo "  Release binary not reachable (HTTP ${HTTP_STATUS}) or private release."
    echo "  Falling back to compiling from source..."
    install_from_source
  fi
else
  install_from_source
fi

chmod +x "${TARGET_BIN_DIR}/lokol"

# 4. PATH Verification
echo
echo "[4/4] Verifying PATH environment..."
LOKOL_PATH_OK=false
IFS=':' read -ra PATH_ENTRIES <<< "${PATH}"
for entry in "${PATH_ENTRIES[@]}"; do
  if [ "${entry}" = "${TARGET_BIN_DIR}" ]; then
    LOKOL_PATH_OK=true
    break
  fi
done

echo
echo "========================================"
echo "          Installation Succeeded!       "
echo "========================================"
echo
echo "Binary Location : ${TARGET_BIN_DIR}/lokol"
echo "Installed Version : $("${TARGET_BIN_DIR}/lokol" version 2>/dev/null || echo "v0.1.0")"
echo

if [ "${LOKOL_PATH_OK}" = false ]; then
  echo "⚠️  NOTE: ${TARGET_BIN_DIR} is not in your current PATH."
  echo "Add the following line to your shell profile (~/.bashrc or ~/.zshrc):"
  echo
  echo "    export PATH=\"\${HOME}/.local/bin:\${PATH}\""
  echo
  echo "Then restart your terminal or run: source ~/.bashrc"
else
  echo "✓ ${TARGET_BIN_DIR} is in your PATH. You can start using lokol immediately:"
  echo
  echo "    lokol --help"
  echo "    lokol chat --yolo"
  echo "    lokol probe"
fi
echo
