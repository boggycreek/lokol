#!/usr/bin/env bash
# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.
#
# lokol Development Environment Setup Script
#
# Automatically detects the platform and installs all required dependencies
# to build, test, and contribute to lokol.
#
# Supported package managers:
#   - macOS: Homebrew (brew)
#   - Linux APT-based: Debian, Ubuntu, Pop!_OS, Linux Mint, etc. (apt-get)
#   - Linux RPM-based: Fedora, RHEL, CentOS, Rocky Linux, AlmaLinux (dnf / yum)
#
# Features:
#   - Multi-version Go SDK installation and management under XDG paths
#   - Environment diagnostic health check (--doctor)
#   - Non-interactive automation (-y / --yes)
#   - Rootless / unprivileged execution (--no-sudo)
#   - Beads issue tracking CLI (bd) bootstrap

set -euo pipefail

# Configuration defaults
NON_INTERACTIVE=false
USE_SUDO=true
SKIP_GO=false
SKIP_BEADS=false
SKIP_CMAKE=false
DRY_RUN=false
RUN_DOCTOR=false
LIST_GO=false
SWITCH_GO=""
REQUESTED_GO_VERSION=""
INSTALL_LLAMA=false
INSTALL_LLAMA_SOURCE=false
SKIP_LLAMA=false

# XDG Base Directory specification paths
XDG_BIN_HOME="${HOME}/.local/bin"
XDG_DATA_HOME="${XDG_DATA_HOME:-${HOME}/.local/share}"
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-${HOME}/.config}"
XDG_CACHE_HOME="${XDG_CACHE_HOME:-${HOME}/.cache}"

GO_SDK_BASE="${XDG_DATA_HOME}/go/sdk"
GO_CURRENT_LINK="${GO_SDK_BASE}/current"

# Resolve default Go version from go.mod if present, otherwise 1.25.8
DEFAULT_GO_VER="1.25.8"
if [ -f "go.mod" ]; then
  DETECTED_MOD_GO="$(grep -E '^go [0-9]' go.mod 2>/dev/null | awk '{print $2}' || echo "")"
  if [ -n "${DETECTED_MOD_GO}" ]; then
    DEFAULT_GO_VER="${DETECTED_MOD_GO}"
  fi
fi

print_usage() {
  cat <<EOF
Usage: setup.sh [options]

Setup development environment dependencies for lokol.

Options:
  -y, --yes, --non-interactive   Run without prompting (assumes 'yes' to package installs)
  --doctor                       Run comprehensive development environment diagnostic checks
  --go-version <version>         Install and activate specific Go SDK version (e.g. 1.25.8)
  --list-go                      List all installed XDG Go SDK versions
  --switch-go <version>          Switch active Go SDK to an already installed version
  --no-sudo                      Do not use sudo (for rootless or unprivileged environments)
  --skip-go                      Skip Go toolchain installation/check
  --skip-beads                   Skip Beads (bd) CLI installation
  --skip-cmake                   Skip CMake build tool installation
  --skip-llama                   Skip llama.cpp / llama-server check or installation
  --install-llama                Download prebuilt or build llama.cpp and llama-server via install-llama.sh
  --install-llama-source         Force compiling llama.cpp and llama-server from source
  --dry-run                      Print actions without executing commands
  -h, --help                     Show this help message
EOF
}

# Parse CLI arguments
while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes|--non-interactive)
      NON_INTERACTIVE=true
      shift
      ;;
    --doctor)
      RUN_DOCTOR=true
      shift
      ;;
    --go-version)
      REQUESTED_GO_VERSION="${2#go}"
      shift 2
      ;;
    --list-go)
      LIST_GO=true
      shift
      ;;
    --switch-go)
      SWITCH_GO="${2#go}"
      shift 2
      ;;
    --no-sudo)
      USE_SUDO=false
      shift
      ;;
    --skip-go)
      SKIP_GO=true
      shift
      ;;
    --skip-beads)
      SKIP_BEADS=true
      shift
      ;;
    --skip-cmake)
      SKIP_CMAKE=true
      shift
      ;;
    --skip-llama)
      SKIP_LLAMA=true
      shift
      ;;
    --install-llama)
      INSTALL_LLAMA=true
      shift
      ;;
    --install-llama-source)
      INSTALL_LLAMA=true
      INSTALL_LLAMA_SOURCE=true
      shift
      ;;
    --dry-run)
      DRY_RUN=true
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

TARGET_GO_VER="${REQUESTED_GO_VERSION:-${DEFAULT_GO_VER}}"

# Detect Platform and Architecture
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

# ---------------------------------------------------------------------------
# Multi-Version Go SDK Management Functions (XDG compliant)
# ---------------------------------------------------------------------------

list_go_sdks() {
  echo "Installed XDG Go SDKs (${GO_SDK_BASE}):"
  if [ ! -d "${GO_SDK_BASE}" ]; then
    echo "  (No Go SDKs installed under ${GO_SDK_BASE})"
    return 0
  fi

  CURRENT_TARGET=""
  if [ -L "${GO_CURRENT_LINK}" ]; then
    CURRENT_TARGET="$(basename "$(readlink -f "${GO_CURRENT_LINK}" 2>/dev/null || readlink "${GO_CURRENT_LINK}")")"
  fi

  FOUND=false
  for dir in "${GO_SDK_BASE}"/go*; do
    if [ -d "${dir}" ]; then
      FOUND=true
      VER_DIR="$(basename "${dir}")"
      if [ "${VER_DIR}" = "${CURRENT_TARGET}" ]; then
        echo "  * ${VER_DIR} [ACTIVE] -> ${dir}"
      else
        echo "    ${VER_DIR}          -> ${dir}"
      fi
    fi
  done

  if [ "${FOUND}" = false ]; then
    echo "  (No Go SDKs found in ${GO_SDK_BASE})"
  fi
}

switch_go_sdk() {
  local ver="${1#go}"
  local target_dir="${GO_SDK_BASE}/go${ver}"

  if [ ! -d "${target_dir}" ] || [ ! -x "${target_dir}/bin/go" ]; then
    echo "Error: Go version go${ver} is not installed under ${GO_SDK_BASE}." >&2
    echo "Install it first using: ./setup.sh --go-version ${ver}" >&2
    exit 1
  fi

  mkdir -p "${XDG_BIN_HOME}"
  mkdir -p "${GO_SDK_BASE}"

  ln -sfn "go${ver}" "${GO_CURRENT_LINK}"
  ln -sfn "${GO_CURRENT_LINK}/bin/go" "${XDG_BIN_HOME}/go"
  ln -sfn "${GO_CURRENT_LINK}/bin/gofmt" "${XDG_BIN_HOME}/gofmt"
  ln -sfn "${target_dir}/bin/go" "${XDG_BIN_HOME}/go${ver}"

  echo "✓ Switched active Go SDK to go${ver}"
  echo "  Active binary: ${XDG_BIN_HOME}/go -> $("${XDG_BIN_HOME}/go" version)"
}

install_xdg_go() {
  local ver="${1#go}"
  local target_dir="${GO_SDK_BASE}/go${ver}"

  echo "  Checking Go SDK go${ver} under XDG path (${GO_SDK_BASE})..."

  if [ -d "${target_dir}" ] && [ -x "${target_dir}/bin/go" ]; then
    echo "  ✓ Go SDK go${ver} is already installed at: ${target_dir}"
  else
    local tarball="go${ver}.${OS}-${ARCH}.tar.gz"
    local url="https://go.dev/dl/${tarball}"
    echo "  Downloading official Go ${ver} from ${url}..."

    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "${tmp_dir}"' RETURN

    if [ "${DRY_RUN}" = true ]; then
      echo "  [DRY-RUN] curl -sSL -o ${tmp_dir}/${tarball} ${url}"
      echo "  [DRY-RUN] extract to ${target_dir}"
    else
      curl -sSL -f -o "${tmp_dir}/${tarball}" "${url}"
      mkdir -p "${GO_SDK_BASE}"
      tar -xzf "${tmp_dir}/${tarball}" -C "${tmp_dir}"
      rm -rf "${target_dir}"
      mv -f "${tmp_dir}/go" "${target_dir}"
      echo "  ✓ Installed Go SDK go${ver} into ${target_dir}"
    fi
  fi

  if [ "${DRY_RUN}" = false ]; then
    mkdir -p "${XDG_BIN_HOME}"
    # Activate this version as current
    ln -sfn "go${ver}" "${GO_CURRENT_LINK}"
    ln -sfn "${GO_CURRENT_LINK}/bin/go" "${XDG_BIN_HOME}/go"
    ln -sfn "${GO_CURRENT_LINK}/bin/gofmt" "${XDG_BIN_HOME}/gofmt"
    # Create version-specific alias (e.g. go1.25.8)
    ln -sfn "${target_dir}/bin/go" "${XDG_BIN_HOME}/go${ver}"
    echo "  ✓ Active Go symlinked to ${XDG_BIN_HOME}/go -> $("${XDG_BIN_HOME}/go" version 2>/dev/null || echo "go${ver}")"
    echo "  ✓ Version-specific binary available at ${XDG_BIN_HOME}/go${ver}"
  fi
}

# ---------------------------------------------------------------------------
# Doctor Diagnostic Check (--doctor)
# ---------------------------------------------------------------------------

run_environment_doctor() {
  echo "=================================================="
  echo "     lokol Development Environment Doctor         "
  echo "=================================================="
  echo

  local passes=0
  local warnings=0
  local failures=0
  local remediation=()

  # 1. Platform
  echo "[1/6] Operating System & Hardware Architecture"
  echo "  [PASS] Platform : ${OS_PRETTY} (${OS}/${ARCH})"
  echo "  [PASS] Kernel   : $(uname -r 2>/dev/null || echo 'Unknown')"
  echo "  [PASS] Hostname : $(hostname 2>/dev/null || echo 'localhost')"
  passes=$((passes + 3))

  # 2. XDG Directory Structure & PATH
  echo
  echo "[2/6] XDG Base Directory Compliance"
  for var_name in XDG_BIN_HOME XDG_DATA_HOME XDG_CONFIG_HOME XDG_CACHE_HOME; do
    local path="${!var_name}"
    if [ -d "${path}" ]; then
      echo "  [PASS] ${var_name}: ${path} (exists)"
      passes=$((passes + 1))
    else
      echo "  [WARN] ${var_name}: ${path} (does not exist yet)"
      warnings=$((warnings + 1))
      remediation+=("mkdir -p \"${path}\"")
    fi
  done

  # PATH verification
  local in_path=false
  IFS=':' read -ra PATH_DIRS <<< "${PATH}"
  for dir in "${PATH_DIRS[@]}"; do
    if [ "${dir}" = "${XDG_BIN_HOME}" ]; then
      in_path=true
      break
    fi
  done

  if [ "${in_path}" = true ]; then
    echo "  [PASS] PATH: ${XDG_BIN_HOME} is properly exported in \$PATH"
    passes=$((passes + 1))
  else
    echo "  [WARN] PATH: ${XDG_BIN_HOME} is NOT currently in \$PATH"
    warnings=$((warnings + 1))
    remediation+=("Add to shell profile: export PATH=\"\${HOME}/.local/bin:\${PATH}\"")
  fi

  # Beads permissions
  if [ -d ".beads" ]; then
    local beads_perms
    beads_perms="$(stat -c "%a" .beads 2>/dev/null || stat -f "%Op" .beads 2>/dev/null || echo "unknown")"
    if [ "${beads_perms}" = "700" ] || [ "${beads_perms}" = "0700" ]; then
      echo "  [PASS] .beads directory permissions: ${beads_perms}"
      passes=$((passes + 1))
    else
      echo "  [WARN] .beads directory permissions: ${beads_perms} (recommended: 0700)"
      warnings=$((warnings + 1))
      remediation+=("chmod 700 .beads")
    fi
  fi

  # 3. Go Toolchain
  echo
  echo "[3/6] Go SDK & Multi-Version Toolchains (XDG)"
  local active_go=""
  if command -v go >/dev/null 2>&1; then
    active_go="$(command -v go)"
  elif [ -x "${XDG_BIN_HOME}/go" ]; then
    active_go="${XDG_BIN_HOME}/go"
  fi

  if [ -n "${active_go}" ] && [ -x "${active_go}" ]; then
    local go_ver_str
    go_ver_str="$("${active_go}" version 2>/dev/null || echo "")"
    local raw_ver
    raw_ver="$(echo "${go_ver_str}" | awk '{print $3}' | sed 's/go//')"
    echo "  [PASS] Active Go: ${go_ver_str} (${active_go})"
    passes=$((passes + 1))

    # Compatibility with go.mod
    echo "  [INFO] Target in go.mod: go ${DEFAULT_GO_VER}"
    # Minimal version check: compare major.minor
    local cur_major_minor
    cur_major_minor="$(echo "${raw_ver}" | cut -d. -f1,2)"
    local target_major_minor
    target_major_minor="$(echo "${DEFAULT_GO_VER}" | cut -d. -f1,2)"

    # Simple numeric comparison of 1.xx
    local cur_minor="${cur_major_minor#1.}"
    local target_minor="${target_major_minor#1.}"
    if [ -n "${cur_minor}" ] && [ -n "${target_minor}" ] && [ "${cur_minor}" -ge "${target_minor}" ] 2>/dev/null; then
      echo "  [PASS] Go compiler version (${raw_ver}) satisfies project requirement (${DEFAULT_GO_VER})"
      passes=$((passes + 1))
    else
      echo "  [WARN] Installed Go (${raw_ver}) is older than go.mod requirement (${DEFAULT_GO_VER})"
      warnings=$((warnings + 1))
      remediation+=("./setup.sh --go-version ${DEFAULT_GO_VER}")
    fi
  else
    echo "  [FAIL] Go compiler not found on PATH or ${XDG_BIN_HOME}/go"
    failures=$((failures + 1))
    remediation+=("./setup.sh --go-version ${DEFAULT_GO_VER}")
  fi

  # List installed XDG Go SDKs
  echo "  [INFO] XDG Go SDK Repository: ${GO_SDK_BASE}"
  if [ -d "${GO_SDK_BASE}" ]; then
    local current_sdk=""
    if [ -L "${GO_CURRENT_LINK}" ]; then
      current_sdk="$(basename "$(readlink -f "${GO_CURRENT_LINK}" 2>/dev/null || readlink "${GO_CURRENT_LINK}")")"
    fi
    local count=0
    for sdir in "${GO_SDK_BASE}"/go*; do
      if [ -d "${sdir}" ]; then
        count=$((count + 1))
        local sname
        sname="$(basename "${sdir}")"
        if [ "${sname}" = "${current_sdk}" ]; then
          echo "         * ${sname} [ACTIVE] (${sdir})"
        else
          echo "           ${sname}          (${sdir})"
        fi
      fi
    done
    if [ "${count}" -eq 0 ]; then
      echo "         (No multi-version SDKs currently installed in ${GO_SDK_BASE})"
    fi
  fi

  # 4. Core Build Tools
  echo
  echo "[4/6] Core Build Tools & Compilers"
  if command -v git >/dev/null 2>&1; then
    echo "  [PASS] git: $(git --version)"
    passes=$((passes + 1))
  else
    echo "  [FAIL] git is missing"
    failures=$((failures + 1))
    remediation+=("Install git via system package manager (./setup.sh)")
  fi

  if command -v make >/dev/null 2>&1; then
    echo "  [PASS] make: $(make --version 2>/dev/null | awk 'NR==1')"
    passes=$((passes + 1))
  else
    echo "  [FAIL] make is missing"
    failures=$((failures + 1))
    remediation+=("Install make via system package manager (./setup.sh)")
  fi

  if command -v gcc >/dev/null 2>&1; then
    echo "  [PASS] C compiler (gcc): $(gcc --version 2>/dev/null | awk 'NR==1')"
    passes=$((passes + 1))
  elif command -v clang >/dev/null 2>&1; then
    echo "  [PASS] C compiler (clang): $(clang --version 2>/dev/null | awk 'NR==1')"
    passes=$((passes + 1))
  else
    echo "  [WARN] C/C++ compiler missing (needed for cgo race detector & native compilation)"
    warnings=$((warnings + 1))
    remediation+=("Install build-essential / gcc via ./setup.sh")
  fi

  if command -v cmake >/dev/null 2>&1; then
    echo "  [PASS] cmake: $(cmake --version 2>/dev/null | awk 'NR==1')"
    passes=$((passes + 1))
  else
    echo "  [WARN] cmake: Not installed (needed if building llama.cpp from source)"
    warnings=$((warnings + 1))
    remediation+=("Install cmake via system package manager or ./setup.sh")
  fi

  if command -v pkg-config >/dev/null 2>&1 || command -v pkgconfig >/dev/null 2>&1; then
    echo "  [PASS] pkg-config: Available"
    passes=$((passes + 1))
  else
    echo "  [WARN] pkg-config: Not installed"
    warnings=$((warnings + 1))
  fi

  # 5. Beads Issue Tracker
  echo
  echo "[5/6] Beads Issue Tracking Tooling (bd)"
  local bd_bin=""
  if command -v bd >/dev/null 2>&1; then
    bd_bin="$(command -v bd)"
  elif [ -x "${XDG_BIN_HOME}/bd" ]; then
    bd_bin="${XDG_BIN_HOME}/bd"
  fi

  if [ -n "${bd_bin}" ]; then
    echo "  [PASS] bd CLI: ${bd_bin} ($("${bd_bin}" --version 2>/dev/null || echo 'installed'))"
    passes=$((passes + 1))
    if [ -d ".beads" ]; then
      local issue_count
      issue_count="$(grep -c '^{"_type":"issue"' .beads/issues.jsonl 2>/dev/null || echo "0")"
      echo "  [PASS] Beads Workspace: Initialized (${issue_count} issues in issues.jsonl)"
      passes=$((passes + 1))
    else
      echo "  [WARN] Beads workspace (.beads) not found in current directory"
      warnings=$((warnings + 1))
    fi
  else
    echo "  [WARN] bd CLI not installed"
    warnings=$((warnings + 1))
    remediation+=("./setup.sh (automatically downloads bd CLI)")
  fi

  # 6. Inference Engine (llama.cpp)
  echo
  echo "[6/6] Inference Engine & llama-server Connectivity"
  local llama_bin=""
  if command -v llama-server >/dev/null 2>&1; then
    llama_bin="$(command -v llama-server)"
  elif command -v llama >/dev/null 2>&1; then
    llama_bin="$(command -v llama)"
  elif [ -x "${XDG_BIN_HOME}/llama-server" ]; then
    llama_bin="${XDG_BIN_HOME}/llama-server"
  elif [ -x "${XDG_BIN_HOME}/llama" ]; then
    llama_bin="${XDG_BIN_HOME}/llama"
  fi

  if [ -n "${llama_bin}" ]; then
    echo "  [PASS] llama binary: ${llama_bin}"
    passes=$((passes + 1))
  else
    echo "  [WARN] llama-server / llama binary not found in PATH or ${XDG_BIN_HOME}"
    echo "         (Install or compile using: ./install-llama.sh)"
    warnings=$((warnings + 1))
    remediation+=("./install-llama.sh (automatically downloads prebuilt or builds llama.cpp)")
  fi

  # Probe local server
  local engine_active=false
  if command -v curl >/dev/null 2>&1; then
    if curl -s -m 1 "http://127.0.0.1:8080/health" >/dev/null 2>&1 || curl -s -m 1 "http://127.0.0.1:8080/slots" >/dev/null 2>&1; then
      engine_active=true
    fi
  fi

  if [ "${engine_active}" = true ]; then
    echo "  [PASS] llama-server: Active and responding on http://127.0.0.1:8080"
    passes=$((passes + 1))
  else
    echo "  [INFO] llama-server: Not currently running on http://127.0.0.1:8080 (optional during unit tests)"
  fi

  # Summary
  echo
  echo "--------------------------------------------------"
  echo "Doctor Diagnostic Summary:"
  echo "  Passed   : ${passes}"
  echo "  Warnings : ${warnings}"
  echo "  Failures : ${failures}"
  echo "--------------------------------------------------"

  if [ ${#remediation[@]} -gt 0 ]; then
    echo
    echo "Suggested Remediation Actions:"
    for rem in "${remediation[@]}"; do
      echo "  - ${rem}"
    done
    echo
  fi

  if [ "${failures}" -gt 0 ]; then
    echo "❌ Doctor check failed with ${failures} critical error(s)."
    exit 1
  else
    echo "✓ Development environment is ready!"
    exit 0
  fi
}

# ---------------------------------------------------------------------------
# Dispatch subcommands
# ---------------------------------------------------------------------------

if [ "${RUN_DOCTOR}" = true ]; then
  run_environment_doctor
fi

if [ "${LIST_GO}" = true ]; then
  list_go_sdks
  exit 0
fi

if [ -n "${SWITCH_GO}" ]; then
  switch_go_sdk "${SWITCH_GO}"
  exit 0
fi

# ---------------------------------------------------------------------------
# Main Setup Flow
# ---------------------------------------------------------------------------

echo "=================================================="
echo "    lokol Development Environment Setup           "
echo "=================================================="
echo

mkdir -p "${XDG_BIN_HOME}"
mkdir -p "${XDG_DATA_HOME}/lokol"
mkdir -p "${XDG_CONFIG_HOME}/lokol"
mkdir -p "${XDG_CACHE_HOME}"
mkdir -p "${GO_SDK_BASE}"

run_cmd() {
  if [ "${DRY_RUN}" = true ]; then
    echo "  [DRY-RUN] $*"
  else
    "$@"
  fi
}

# 1. Platform Detection
echo "[1/6] Detecting operating system and architecture..."
echo "  Platform: ${OS_PRETTY} (${OS}/${ARCH})"

# Determine Package Manager
PKG_MGR=""
if [ "${OS}" = "darwin" ]; then
  if command -v brew >/dev/null 2>&1; then
    PKG_MGR="brew"
  else
    PKG_MGR="missing_brew"
  fi
elif [ "${OS}" = "linux" ]; then
  if command -v apt-get >/dev/null 2>&1; then
    PKG_MGR="apt"
  elif command -v dnf >/dev/null 2>&1; then
    PKG_MGR="dnf"
  elif command -v yum >/dev/null 2>&1; then
    PKG_MGR="yum"
  fi
fi

if [ -z "${PKG_MGR}" ]; then
  echo "Error: Could not detect a supported package manager (brew, apt-get, dnf, yum)." >&2
  echo "Please install dependencies manually. See CONTRIBUTING.md for required packages." >&2
  exit 1
fi

echo "  Package Manager: ${PKG_MGR}"

# Privilege Elevation Setup
SUDO=""
if [ "${OS}" = "linux" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
  elif [ "${USE_SUDO}" = true ]; then
    if command -v sudo >/dev/null 2>&1; then
      SUDO="sudo"
    else
      echo "Warning: Running as non-root and sudo was not found." >&2
      echo "System package installation may require root privileges." >&2
    fi
  fi
fi

# 2. Package Manager Dependencies Installation
echo
echo "[2/6] Installing development toolchain packages..."

case "${PKG_MGR}" in
  brew)
    echo "  Configuring Homebrew environment..."
    export HOMEBREW_NO_AUTO_UPDATE=1

    BREW_PACKAGES=(git curl make pkg-config)
    if [ "${SKIP_CMAKE}" = false ]; then
      BREW_PACKAGES+=(cmake)
    fi

    echo "  Installing Homebrew packages: ${BREW_PACKAGES[*]}..."
    run_cmd brew install "${BREW_PACKAGES[@]}"
    ;;

  missing_brew)
    echo "  ⚠️  Homebrew (brew) is not installed on macOS."
    echo "  Please install Homebrew by running:"
    echo '    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
    echo "  Then re-run ./setup.sh"
    exit 1
    ;;

  apt)
    echo "  Updating APT package lists..."
    if [ -n "${SUDO}" ] || [ "$(id -u)" -eq 0 ]; then
      run_cmd ${SUDO} apt-get update -y
      APT_PACKAGES=(curl git make build-essential pkg-config tar gzip ca-certificates)
      if [ "${SKIP_CMAKE}" = false ]; then
        APT_PACKAGES+=(cmake)
      fi
      echo "  Installing APT packages: ${APT_PACKAGES[*]}..."
      run_cmd ${SUDO} apt-get install -y "${APT_PACKAGES[@]}"
    else
      echo "  ⚠️  Skipping APT package installation (no root/sudo privileges)."
      echo "     Ensure git, make, gcc, and curl are installed."
    fi
    ;;

  dnf|yum)
    echo "  Installing RPM packages with ${PKG_MGR}..."
    if [ -n "${SUDO}" ] || [ "$(id -u)" -eq 0 ]; then
      RPM_PACKAGES=(curl git make gcc gcc-c++ pkgconfig tar gzip ca-certificates)
      if [ "${SKIP_CMAKE}" = false ]; then
        RPM_PACKAGES+=(cmake)
      fi
      echo "  Installing RPM packages: ${RPM_PACKAGES[*]}..."
      run_cmd ${SUDO} "${PKG_MGR}" install -y "${RPM_PACKAGES[@]}"
    else
      echo "  ⚠️  Skipping RPM package installation (no root/sudo privileges)."
      echo "     Ensure git, make, gcc, and curl are installed."
    fi
    ;;
esac

# 3. Multi-Version Go Toolchain Setup (under XDG)
echo
echo "[3/6] Setting up Go toolchain under XDG path (${GO_SDK_BASE})..."

if [ "${SKIP_GO}" = false ]; then
  install_xdg_go "${TARGET_GO_VER}"
else
  echo "  Skipping Go installation (--skip-go)."
fi

# 4. Beads Issue Tracker CLI (bd) Setup
echo
echo "[4/6] Setting up Beads (bd) issue tracking CLI..."
if [ "${SKIP_BEADS}" = false ]; then
  if command -v bd >/dev/null 2>&1; then
    echo "  ✓ Found bd at: $(command -v bd)"
  elif [ -x "${XDG_BIN_HOME}/bd" ]; then
    echo "  ✓ Found bd at: ${XDG_BIN_HOME}/bd"
  else
    echo "  Downloading bd binary release (gastownhall/beads)..."
    BEADS_VER="1.3.0"
    BEADS_TARBALL="beads_${BEADS_VER}_${OS}_${ARCH}.tar.gz"
    BEADS_URL="https://github.com/gastownhall/beads/releases/download/v${BEADS_VER}/${BEADS_TARBALL}"

    TMP_BEADS="$(mktemp -d)"
    trap 'rm -rf "${TMP_BEADS}"' EXIT

    if curl -sSL -f -o "${TMP_BEADS}/${BEADS_TARBALL}" "${BEADS_URL}"; then
      tar -xzf "${TMP_BEADS}/${BEADS_TARBALL}" -C "${TMP_BEADS}" bd
      cp -f "${TMP_BEADS}/bd" "${XDG_BIN_HOME}/bd"
      chmod +x "${XDG_BIN_HOME}/bd"
      echo "  ✓ Installed bd to ${XDG_BIN_HOME}/bd"
    else
      echo "  ⚠️  Could not automatically download bd. You can install it manually from:"
      echo "     https://github.com/gastownhall/beads/releases"
    fi
  fi
else
  echo "  Skipping Beads CLI setup (--skip-beads)."
fi

# 5. Inference Engine (llama.cpp) Status
echo
echo "[5/6] Checking inference engine status..."
LLAMA_FOUND=false
if command -v llama-server >/dev/null 2>&1; then
  echo "  ✓ Found llama-server at: $(command -v llama-server)"
  LLAMA_FOUND=true
elif command -v llama >/dev/null 2>&1; then
  echo "  ✓ Found llama at: $(command -v llama)"
  LLAMA_FOUND=true
elif [ -x "${XDG_BIN_HOME}/llama-server" ] || [ -x "${XDG_BIN_HOME}/llama" ]; then
  echo "  ✓ Found llama binary in ${XDG_BIN_HOME}"
  LLAMA_FOUND=true
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"
LLAMA_INSTALLER="${SCRIPT_DIR}/install-llama.sh"
if [ ! -f "${LLAMA_INSTALLER}" ]; then
  LLAMA_INSTALLER="./install-llama.sh"
fi

if [ "${SKIP_LLAMA}" = true ]; then
  echo "  Skipping inference engine setup (--skip-llama)."
elif [ "${INSTALL_LLAMA}" = true ]; then
  if [ -f "${LLAMA_INSTALLER}" ]; then
    echo "  Invoking llama.cpp installer (${LLAMA_INSTALLER})..."
    INSTALL_ARGS=("-y")
    if [ "${INSTALL_LLAMA_SOURCE}" = true ]; then
      INSTALL_ARGS+=("--build-from-source")
    fi
    if [ "${USE_SUDO}" = false ]; then
      INSTALL_ARGS+=("--no-sudo")
    fi
    if [ "${DRY_RUN}" = true ]; then
      INSTALL_ARGS+=("--dry-run")
    fi
    "${LLAMA_INSTALLER}" "${INSTALL_ARGS[@]}"
  else
    echo "  ⚠️  Installer script not found: ${LLAMA_INSTALLER}"
  fi
elif [ "${LLAMA_FOUND}" = false ]; then
  echo "  ℹ️  llama-server is not currently detected."
  echo "     lokol connects to a local llama-server instance (default: http://127.0.0.1:8080)."
  echo "     Install or build it automatically using: ./install-llama.sh"
fi

# 6. Environment and PATH Verification
echo
echo "[6/6] Verifying PATH environment..."
PATH_OK=false
IFS=':' read -ra PATH_DIRS <<< "${PATH}"
for dir in "${PATH_DIRS[@]}"; do
  if [ "${dir}" = "${XDG_BIN_HOME}" ]; then
    PATH_OK=true
    break
  fi
done

echo
echo "=================================================="
echo "    Development Environment Ready!                "
echo "=================================================="
echo

# Summary table
echo "Tool Status:"
echo "  - OS / Arch   : ${OS_PRETTY} (${OS}/${ARCH})"
echo "  - Git         : $(if command -v git >/dev/null 2>&1; then git --version; else echo 'Not installed'; fi)"
echo "  - Make        : $(if command -v make >/dev/null 2>&1; then make --version 2>/dev/null | awk 'NR==1'; else echo 'Not installed'; fi)"
echo "  - C Compiler  : $(if command -v gcc >/dev/null 2>&1; then gcc --version 2>/dev/null | awk 'NR==1'; elif command -v clang >/dev/null 2>&1; then clang --version 2>/dev/null | awk 'NR==1'; else echo 'Not installed'; fi)"
echo "  - CMake       : $(if command -v cmake >/dev/null 2>&1; then cmake --version 2>/dev/null | awk 'NR==1'; else echo 'Not installed'; fi)"
echo "  - Go          : $(if command -v go >/dev/null 2>&1; then go version; elif [ -x "${XDG_BIN_HOME}/go" ]; then "${XDG_BIN_HOME}/go" version; else echo 'Not installed'; fi)"
echo "  - Beads (bd)  : $(if command -v bd >/dev/null 2>&1; then echo "Installed ($("${bd_bin:-bd}" --version 2>/dev/null || echo 'detected'))"; elif [ -x "${XDG_BIN_HOME}/bd" ]; then echo "Installed in ~/.local/bin"; else echo 'Not installed'; fi)"
echo

if [ "${PATH_OK}" = false ]; then
  echo "⚠️  NOTE: ${XDG_BIN_HOME} is not currently in your PATH."
  echo "Add the following line to your shell profile (~/.bashrc or ~/.zshrc):"
  echo
  echo "    export PATH=\"\${HOME}/.local/bin:\${PATH}\""
  echo
  echo "Then reload your shell: source ~/.bashrc (or source ~/.zshrc)"
  echo
fi

echo "Next steps:"
echo "  1. Verify your environment with doctor:"
echo "     ./setup.sh --doctor"
echo
echo "  2. Run the test suite:"
echo "     make test"
echo
echo "  3. Build the lokol binary:"
echo "     make build"
echo
echo "  4. Inspect open tasks with Beads:"
echo "     bd ready"
echo
