#!/usr/bin/env bash
# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.
#
# lokol llama.cpp & llama-server Multi-OS Installer and Builder
#
# Automatically downloads prebuilt binaries or compiles llama.cpp / llama-server
# from source across supported platforms:
#   - macOS (Darwin): Apple Silicon (Metal) / Intel x86_64
#   - Linux APT-based: Debian, Ubuntu, Pop!_OS, Linux Mint (apt-get)
#   - Linux RPM-based: Fedora, RHEL, CentOS, Rocky Linux, AlmaLinux (dnf / yum)
#
# Hardware Acceleration Support:
#   - Apple Metal (-DGGML_METAL=ON)
#   - NVIDIA CUDA (-DGGML_CUDA=ON)
#   - Vulkan API  (-DGGML_VULKAN=ON)
#   - CPU Vector Extensions (AVX2, AVX-512, NEON via -DGGML_NATIVE=ON)
#
# Non-Interactive Automation:
#   Always uses non-interactive flags for file and package management operations.

set -euo pipefail

# Configuration defaults
MODE="auto" # "auto", "prebuilt", "source", "brew"
REQUESTED_TAG="latest"
BACKEND="auto" # "auto", "cpu", "cuda", "vulkan", "metal"
NON_INTERACTIVE=false
USE_SUDO=true
DRY_RUN=false
CHECK_STATUS=false
USE_BREW=false

# XDG Base Directory specification paths
XDG_BIN_HOME="${XDG_BIN_HOME:-${HOME}/.local/bin}"
XDG_DATA_HOME="${XDG_DATA_HOME:-${HOME}/.local/share}"
XDG_CACHE_HOME="${XDG_CACHE_HOME:-${HOME}/.cache}"

TARGET_BIN_DIR="${XDG_BIN_HOME}"
INSTALL_DIR="${XDG_DATA_HOME}/llama.cpp"
BUILD_CACHE_DIR="${XDG_CACHE_HOME}/lokol/llama.cpp"

REPO_OWNER="ggml-org"
REPO_NAME="llama.cpp"
REPO_GIT_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}.git"

# Fallback known release tag if GitHub API rate-limited
FALLBACK_TAG="b11126"

print_usage() {
  cat <<'EOF'
Usage: install-llama.sh [options]

Install or compile llama.cpp and llama-server for lokol.

Options:
  --prebuilt                   Download official prebuilt binaries from GitHub Releases (default)
  --build-from-source, --source Compile llama.cpp and llama-server from source via CMake
  --use-brew                   Use Homebrew to install llama.cpp on macOS (brew install llama.cpp)
  --version <tag>, --tag <tag> Release tag or build number to install (e.g. b11126, or 'latest')
  --backend <backend>          Hardware acceleration backend: auto, cpu, cuda, vulkan, metal
  --bin-dir <path>             Target directory for executable symlinks (default: ~/.local/bin)
  --install-dir <path>         Target directory for binary bundle & libraries (default: ~/.local/share/llama.cpp)
  --status, --check            Inspect current llama.cpp / llama-server installation and health
  --no-sudo                    Do not use sudo for installing system package dependencies
  -y, --yes, --non-interactive Run without interactive prompts
  --dry-run                    Preview installation actions without modifying the system
  -h, --help                   Show this help message

Examples:
  ./install-llama.sh                                # Automatic prebuilt download & setup
  ./install-llama.sh --build-from-source            # Build from source with auto-detected GPU
  ./install-llama.sh --backend cuda                 # Force NVIDIA CUDA GPU acceleration
  ./install-llama.sh --status                       # Verify existing engine and connectivity
EOF
}

# Parse CLI arguments
while [ $# -gt 0 ]; do
  case "$1" in
    --prebuilt)
      MODE="prebuilt"
      shift
      ;;
    --build-from-source|--build|--source)
      MODE="source"
      shift
      ;;
    --use-brew)
      USE_BREW=true
      shift
      ;;
    --version|--tag)
      REQUESTED_TAG="$2"
      shift 2
      ;;
    --backend)
      BACKEND="$2"
      shift 2
      ;;
    --bin-dir)
      TARGET_BIN_DIR="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --status|--check)
      CHECK_STATUS=true
      shift
      ;;
    --no-sudo)
      USE_SUDO=false
      shift
      ;;
    -y|--yes|--non-interactive)
      NON_INTERACTIVE=true
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
    ARCH="x64"
    ARCH_ALT="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ARCH_ALT="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture: ${ARCH_RAW}" >&2
    exit 1
    ;;
esac

# Package Manager Detection
PKG_MGR=""
if [ "${OS}" = "darwin" ]; then
  if command -v brew >/dev/null 2>&1; then
    PKG_MGR="brew"
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

# Privilege Elevation Setup
SUDO=""
if [ "${OS}" = "linux" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
  elif [ "${USE_SUDO}" = true ]; then
    if command -v sudo >/dev/null 2>&1; then
      SUDO="sudo"
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Status Inspection (--status)
# ---------------------------------------------------------------------------

inspect_status() {
  echo "=================================================="
  echo "     llama.cpp & llama-server Diagnostic Status   "
  echo "=================================================="
  echo
  echo "Host Platform: ${OS_PRETTY} (${OS}/${ARCH})"
  echo "Target Bin   : ${TARGET_BIN_DIR}"
  echo "Install Dir  : ${INSTALL_DIR}"
  echo

  local server_bin=""
  local cli_bin=""

  if command -v llama-server >/dev/null 2>&1; then
    server_bin="$(command -v llama-server)"
  elif [ -x "${TARGET_BIN_DIR}/llama-server" ]; then
    server_bin="${TARGET_BIN_DIR}/llama-server"
  elif [ -x "${INSTALL_DIR}/llama-server" ]; then
    server_bin="${INSTALL_DIR}/llama-server"
  fi

  if command -v llama-cli >/dev/null 2>&1; then
    cli_bin="$(command -v llama-cli)"
  elif command -v llama >/dev/null 2>&1; then
    cli_bin="$(command -v llama)"
  elif [ -x "${TARGET_BIN_DIR}/llama-cli" ]; then
    cli_bin="${TARGET_BIN_DIR}/llama-cli"
  elif [ -x "${TARGET_BIN_DIR}/llama" ]; then
    cli_bin="${TARGET_BIN_DIR}/llama"
  elif [ -x "${INSTALL_DIR}/llama-cli" ]; then
    cli_bin="${INSTALL_DIR}/llama-cli"
  fi

  if [ -n "${server_bin}" ]; then
    echo "  [PASS] llama-server: ${server_bin}"
    local ver_out
    ver_out="$("${server_bin}" --version 2>&1 | grep -i 'version:' | head -n 1 || true)"
    if [ -z "${ver_out}" ]; then
      ver_out="$("${server_bin}" --version 2>&1 | head -n 2 | tr '\n' ' ' || echo "unknown")"
    fi
    echo "         Build Info  : ${ver_out}"
  else
    echo "  [FAIL] llama-server: Not found in PATH or ${TARGET_BIN_DIR}"
  fi

  if [ -n "${cli_bin}" ]; then
    echo "  [PASS] llama CLI   : ${cli_bin}"
  else
    echo "  [WARN] llama CLI   : Not found (optional if llama-server is present)"
  fi

  # Service probe
  echo
  echo "Engine Health Probe (http://127.0.0.1:8080):"
  local active=false
  if command -v curl >/dev/null 2>&1; then
    if curl -s -m 1 "http://127.0.0.1:8080/health" >/dev/null 2>&1 || curl -s -m 1 "http://127.0.0.1:8080/slots" >/dev/null 2>&1; then
      active=true
    fi
  fi

  if [ "${active}" = true ]; then
    echo "  [PASS] Service Status: Active and responding on port 8080"
  else
    echo "  [INFO] Service Status: Offline (service is not currently listening)"
  fi

  # Hardware acceleration probe
  echo
  echo "Hardware Acceleration Capabilities:"
  if [ "${OS}" = "darwin" ]; then
    if [ "${ARCH}" = "arm64" ]; then
      echo "  [PASS] Apple Silicon Metal acceleration supported"
    else
      echo "  [INFO] macOS Intel (Accelerate framework / CPU)"
    fi
  elif [ "${OS}" = "linux" ]; then
    if command -v nvidia-smi >/dev/null 2>&1; then
      local gpu_name
      gpu_name="$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -n 1 || echo "NVIDIA GPU")"
      echo "  [PASS] NVIDIA GPU Detected: ${gpu_name} (CUDA support available)"
      if command -v nvcc >/dev/null 2>&1; then
        echo "         nvcc compiler: $(command -v nvcc) ($(nvcc --version 2>/dev/null | grep 'release' || echo ''))"
      fi
    elif [ -d "/proc/driver/nvidia" ]; then
      echo "  [PASS] NVIDIA Driver Detected (/proc/driver/nvidia)"
    else
      echo "  [INFO] No NVIDIA GPU detected"
    fi

    if command -v vulkaninfo >/dev/null 2>&1 || [ -f "/usr/include/vulkan/vulkan.h" ]; then
      echo "  [PASS] Vulkan API available for cross-vendor GPU acceleration"
    fi
  fi
  echo
}

if [ "${CHECK_STATUS}" = true ]; then
  inspect_status
  exit 0
fi

# ---------------------------------------------------------------------------
# Backend Resolution
# ---------------------------------------------------------------------------

resolve_backend() {
  if [ "${BACKEND}" != "auto" ]; then
    case "${BACKEND}" in
      cpu|cuda|vulkan|metal)
        ;;
      *)
        echo "Error: Unsupported backend '${BACKEND}'. Supported: auto, cpu, cuda, vulkan, metal" >&2
        exit 1
        ;;
    esac
    if [ "${BACKEND}" = "metal" ] && [ "${OS}" != "darwin" ]; then
      echo "Error: Metal backend is only supported on macOS." >&2
      exit 1
    fi
    echo "${BACKEND}"
    return 0
  fi

  if [ "${OS}" = "darwin" ]; then
    if [ "${ARCH}" = "arm64" ]; then
      echo "metal"
    else
      echo "cpu"
    fi
    return 0
  fi

  # Linux auto-detection
  if command -v nvidia-smi >/dev/null 2>&1 || [ -d "/proc/driver/nvidia" ] || command -v nvcc >/dev/null 2>&1; then
    echo "cuda"
    return 0
  fi

  if command -v vulkaninfo >/dev/null 2>&1; then
    echo "vulkan"
    return 0
  fi

  echo "cpu"
}

RESOLVED_BACKEND="$(resolve_backend)"

# ---------------------------------------------------------------------------
# Helper: Package Dependency Installation for Building from Source
# ---------------------------------------------------------------------------

install_build_prerequisites() {
  echo "  Checking and installing build prerequisites..."
  case "${PKG_MGR}" in
    brew)
      export HOMEBREW_NO_AUTO_UPDATE=1
      local brew_pkgs=(cmake git pkg-config)
      echo "  Installing Homebrew build packages: ${brew_pkgs[*]}..."
      brew install "${brew_pkgs[@]}"
      ;;
    apt)
      if [ -n "${SUDO}" ] || [ "$(id -u)" -eq 0 ]; then
        echo "  Updating APT lists and installing build packages..."
        ${SUDO} apt-get update -y
        local apt_pkgs=(git cmake build-essential pkg-config curl tar gzip ca-certificates)
        if [ "${RESOLVED_BACKEND}" = "vulkan" ]; then
          apt_pkgs+=(libvulkan-dev vulkan-tools)
        fi
        ${SUDO} apt-get install -y "${apt_pkgs[@]}"
      else
        echo "  ⚠️  Running without sudo; assuming compiler and cmake are installed."
      fi
      ;;
    dnf|yum)
      if [ -n "${SUDO}" ] || [ "$(id -u)" -eq 0 ]; then
        echo "  Installing RPM build packages with ${PKG_MGR}..."
        local rpm_pkgs=(git cmake gcc gcc-c++ make pkgconfig curl tar gzip ca-certificates)
        if [ "${RESOLVED_BACKEND}" = "vulkan" ]; then
          rpm_pkgs+=(vulkan-headers vulkan-loader-devel)
        fi
        ${SUDO} "${PKG_MGR}" install -y "${rpm_pkgs[@]}"
      else
        echo "  ⚠️  Running without sudo; assuming compiler and cmake are installed."
      fi
      ;;
    *)
      echo "  ⚠️  No recognized package manager. Ensure cmake, git, and a C/C++ compiler are installed."
      ;;
  esac
}

# ---------------------------------------------------------------------------
# Release Tag Resolution
# ---------------------------------------------------------------------------

resolve_release_tag() {
  if [ "${REQUESTED_TAG}" != "latest" ]; then
    echo "${REQUESTED_TAG}"
    return 0
  fi

  local resolved=""
  if command -v curl >/dev/null 2>&1; then
    # Try fetching latest release from GitHub API
    local api_url="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases"
    resolved="$(curl -sSL -m 5 "${api_url}" 2>/dev/null | grep '"tag_name":' | head -n 1 | cut -d '"' -f 4 || echo "")"
    if [ -z "${resolved}" ]; then
      # Fallback to redirect location of releases/latest
      local redirect_url
      redirect_url="$(curl -sSI -m 5 "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/latest" 2>/dev/null | grep -i '^location:' | head -n 1 | awk '{print $2}' | tr -d '\r\n' || echo "")"
      if [ -n "${redirect_url}" ]; then
        resolved="$(basename "${redirect_url}")"
      fi
    fi
  fi

  if [ -z "${resolved}" ]; then
    resolved="${FALLBACK_TAG}"
  fi

  echo "${resolved}"
}

# ---------------------------------------------------------------------------
# Prebuilt Binary Installation
# ---------------------------------------------------------------------------

install_prebuilt() {
  local tag="$1"
  echo "  Attempting prebuilt binary installation (release: ${tag}, backend: ${RESOLVED_BACKEND})..."

  local asset_name=""
  if [ "${OS}" = "darwin" ]; then
    if [ "${ARCH}" = "arm64" ]; then
      asset_name="llama-${tag}-bin-macos-arm64.tar.gz"
    else
      asset_name="llama-${tag}-bin-macos-x64.tar.gz"
    fi
  elif [ "${OS}" = "linux" ]; then
    if [ "${RESOLVED_BACKEND}" = "vulkan" ]; then
      asset_name="llama-${tag}-bin-ubuntu-vulkan-${ARCH}.tar.gz"
    elif [ "${RESOLVED_BACKEND}" = "cuda" ]; then
      # Query releases to determine CUDA version asset if possible
      local cuda_asset=""
      if command -v curl >/dev/null 2>&1; then
        cuda_asset="$(curl -sSL -m 5 "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/tags/${tag}" 2>/dev/null | grep '"name":' | grep "llama-${tag}-bin-ubuntu-cuda-" | grep "${ARCH}.tar.gz" | head -n 1 | cut -d '"' -f 4 || echo "")"
      fi
      if [ -n "${cuda_asset}" ]; then
        asset_name="${cuda_asset}"
      else
        # Fall back to Vulkan or standard Ubuntu build
        asset_name="llama-${tag}-bin-ubuntu-${ARCH}.tar.gz"
      fi
    else
      asset_name="llama-${tag}-bin-ubuntu-${ARCH}.tar.gz"
    fi
  fi

  if [ -z "${asset_name}" ]; then
    echo "  ⚠️  Could not determine matching prebuilt asset name."
    return 1
  fi

  local download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}/${asset_name}"
  echo "  Downloading ${asset_name} from GitHub Releases..."

  local tmp_download_dir
  tmp_download_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_download_dir}"' RETURN

  local tarball_path="${tmp_download_dir}/${asset_name}"
  local http_code
  http_code="$(curl -sSL -w "%{http_code}" -o "${tarball_path}" "${download_url}" || echo "000")"

  if [ "${http_code}" != "200" ] || [ ! -s "${tarball_path}" ]; then
    echo "  ⚠️  Prebuilt asset download failed (HTTP ${http_code}): ${download_url}"
    # If a specific CUDA/Vulkan flavor failed, try the standard release asset
    if [ "${RESOLVED_BACKEND}" != "cpu" ] && [ "${OS}" = "linux" ]; then
      local fallback_asset="llama-${tag}-bin-ubuntu-${ARCH}.tar.gz"
      local fallback_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}/${fallback_asset}"
      echo "  Attempting fallback to standard Linux prebuilt release: ${fallback_asset}..."
      http_code="$(curl -sSL -w "%{http_code}" -o "${tarball_path}" "${fallback_url}" || echo "000")"
      if [ "${http_code}" != "200" ] || [ ! -s "${tarball_path}" ]; then
        return 1
      fi
    else
      return 1
    fi
  fi

  echo "  Extracting bundle into ${INSTALL_DIR}..."
  mkdir -p "${INSTALL_DIR}"
  mkdir -p "${TARGET_BIN_DIR}"

  tar -xzf "${tarball_path}" -C "${INSTALL_DIR}" --strip-components=1

  # Create symlinks in TARGET_BIN_DIR
  if [ -x "${INSTALL_DIR}/llama-server" ]; then
    ln -sfn "${INSTALL_DIR}/llama-server" "${TARGET_BIN_DIR}/llama-server"
    echo "  ✓ Linked ${TARGET_BIN_DIR}/llama-server -> ${INSTALL_DIR}/llama-server"
  fi

  if [ -x "${INSTALL_DIR}/llama-cli" ]; then
    ln -sfn "${INSTALL_DIR}/llama-cli" "${TARGET_BIN_DIR}/llama-cli"
    echo "  ✓ Linked ${TARGET_BIN_DIR}/llama-cli -> ${INSTALL_DIR}/llama-cli"
    # Also link llama -> llama-cli if llama doesn't exist as a regular file
    if [ ! -f "${TARGET_BIN_DIR}/llama" ] || [ -L "${TARGET_BIN_DIR}/llama" ]; then
      ln -sfn "${INSTALL_DIR}/llama-cli" "${TARGET_BIN_DIR}/llama"
      echo "  ✓ Linked ${TARGET_BIN_DIR}/llama -> ${INSTALL_DIR}/llama-cli"
    fi
  fi

  # Verify executable execution
  if [ -x "${TARGET_BIN_DIR}/llama-server" ]; then
    local test_run
    if test_run="$("${TARGET_BIN_DIR}/llama-server" --version 2>&1 | head -n 2)"; then
      echo "  ✓ Prebuilt llama-server verified successfully: ${test_run}"
      return 0
    else
      echo "  ⚠️  llama-server test execution failed (shared library incompatibility)."
      return 1
    fi
  fi

  return 1
}

# ---------------------------------------------------------------------------
# Homebrew Installation (macOS)
# ---------------------------------------------------------------------------

install_via_brew() {
  if [ "${OS}" != "darwin" ]; then
    echo "Error: Homebrew installation option is only applicable to macOS." >&2
    return 1
  fi

  if ! command -v brew >/dev/null 2>&1; then
    echo "Error: Homebrew (brew) is not installed." >&2
    return 1
  fi

  echo "  Installing llama.cpp formula via Homebrew..."
  export HOMEBREW_NO_AUTO_UPDATE=1
  brew install llama.cpp

  mkdir -p "${TARGET_BIN_DIR}"

  local brew_prefix
  brew_prefix="$(brew --prefix llama.cpp 2>/dev/null || echo "")"
  if [ -n "${brew_prefix}" ] && [ -d "${brew_prefix}/bin" ]; then
    if [ -x "${brew_prefix}/bin/llama-server" ]; then
      ln -sfn "${brew_prefix}/bin/llama-server" "${TARGET_BIN_DIR}/llama-server"
    fi
    if [ -x "${brew_prefix}/bin/llama-cli" ]; then
      ln -sfn "${brew_prefix}/bin/llama-cli" "${TARGET_BIN_DIR}/llama-cli"
      if [ ! -f "${TARGET_BIN_DIR}/llama" ] || [ -L "${TARGET_BIN_DIR}/llama" ]; then
        ln -sfn "${brew_prefix}/bin/llama-cli" "${TARGET_BIN_DIR}/llama"
      fi
    fi
    echo "  ✓ Successfully linked Homebrew llama.cpp binaries into ${TARGET_BIN_DIR}"
    return 0
  fi

  return 1
}

# ---------------------------------------------------------------------------
# Build from Source
# ---------------------------------------------------------------------------

build_from_source() {
  local tag="$1"
  echo "  Building llama.cpp from source (release/tag: ${tag}, backend: ${RESOLVED_BACKEND})..."

  install_build_prerequisites

  mkdir -p "${BUILD_CACHE_DIR}"
  local src_dir="${BUILD_CACHE_DIR}/src"
  local build_dir="${BUILD_CACHE_DIR}/build"

  if [ -d "${src_dir}/.git" ]; then
    echo "  Updating existing llama.cpp source checkout in ${src_dir}..."
    (
      cd "${src_dir}"
      git fetch --tags --depth 1 origin
      if [ "${tag}" != "latest" ]; then
        git checkout "${tag}"
      else
        git checkout master || git checkout main
        git pull --ff-only || true
      fi
    )
  else
    echo "  Cloning ${REPO_GIT_URL} into ${src_dir}..."
    rm -rf "${src_dir}"
    if [ "${tag}" != "latest" ]; then
      git clone --depth 1 --branch "${tag}" "${REPO_GIT_URL}" "${src_dir}"
    else
      git clone --depth 1 "${REPO_GIT_URL}" "${src_dir}"
    fi
  fi

  local cmake_flags=(
    "-DCMAKE_BUILD_TYPE=Release"
    "-DBUILD_SHARED_LIBS=ON"
  )

  case "${RESOLVED_BACKEND}" in
    metal)
      echo "  Enabling Apple Metal acceleration (-DGGML_METAL=ON)..."
      cmake_flags+=("-DGGML_METAL=ON")
      ;;
    cuda)
      echo "  Enabling NVIDIA CUDA acceleration (-DGGML_CUDA=ON)..."
      cmake_flags+=("-DGGML_CUDA=ON")
      ;;
    vulkan)
      echo "  Enabling Vulkan acceleration (-DGGML_VULKAN=ON)..."
      cmake_flags+=("-DGGML_VULKAN=ON")
      ;;
    cpu|*)
      echo "  Configuring CPU optimization (-DGGML_NATIVE=ON)..."
      cmake_flags+=("-DGGML_NATIVE=ON")
      ;;
  esac

  local num_jobs
  num_jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"

  echo "  Running CMake configuration..."
  rm -rf "${build_dir}"
  cmake -B "${build_dir}" -S "${src_dir}" "${cmake_flags[@]}"

  echo "  Compiling llama-server and llama-cli (${num_jobs} parallel jobs)..."
  cmake --build "${build_dir}" --config Release -j "${num_jobs}" --target llama-server llama-cli

  echo "  Installing compiled binaries into ${INSTALL_DIR}..."
  mkdir -p "${INSTALL_DIR}"
  mkdir -p "${TARGET_BIN_DIR}"

  # Copy compiled executables and libraries
  find "${build_dir}/bin" -maxdepth 1 -type f -perm /111 -exec cp -f {} "${INSTALL_DIR}/" \;
  find "${build_dir}" -maxdepth 2 -type f \( -name "*.so*" -o -name "*.dylib*" \) -exec cp -f {} "${INSTALL_DIR}/" \; 2>/dev/null || true

  # Symlink to TARGET_BIN_DIR
  if [ -x "${INSTALL_DIR}/llama-server" ]; then
    ln -sfn "${INSTALL_DIR}/llama-server" "${TARGET_BIN_DIR}/llama-server"
    echo "  ✓ Linked ${TARGET_BIN_DIR}/llama-server -> ${INSTALL_DIR}/llama-server"
  fi

  if [ -x "${INSTALL_DIR}/llama-cli" ]; then
    ln -sfn "${INSTALL_DIR}/llama-cli" "${TARGET_BIN_DIR}/llama-cli"
    echo "  ✓ Linked ${TARGET_BIN_DIR}/llama-cli -> ${INSTALL_DIR}/llama-cli"
    if [ ! -f "${TARGET_BIN_DIR}/llama" ] || [ -L "${TARGET_BIN_DIR}/llama" ]; then
      ln -sfn "${INSTALL_DIR}/llama-cli" "${TARGET_BIN_DIR}/llama"
      echo "  ✓ Linked ${TARGET_BIN_DIR}/llama -> ${INSTALL_DIR}/llama-cli"
    fi
  fi

  # Verification
  if [ -x "${TARGET_BIN_DIR}/llama-server" ]; then
    local test_run
    if test_run="$("${TARGET_BIN_DIR}/llama-server" --version 2>&1 | head -n 2)"; then
      echo "  ✓ Built llama-server verified successfully: ${test_run}"
      return 0
    fi
  fi

  return 1
}

# ---------------------------------------------------------------------------
# Main Execution Flow
# ---------------------------------------------------------------------------

echo "=================================================="
echo "    lokol llama.cpp & llama-server Installer      "
echo "=================================================="
echo "Platform : ${OS_PRETTY} (${OS}/${ARCH})"
echo "Backend  : ${RESOLVED_BACKEND}"
echo "Bin Dir  : ${TARGET_BIN_DIR}"
echo

if [ "${DRY_RUN}" = true ]; then
  echo "[DRY-RUN] Selected mode: ${MODE}"
  echo "[DRY-RUN] Target release: ${REQUESTED_TAG}"
  echo "[DRY-RUN] Backend: ${RESOLVED_BACKEND}"
  echo "[DRY-RUN] Target Bin: ${TARGET_BIN_DIR}"
  echo "[DRY-RUN] Install Dir: ${INSTALL_DIR}"
  exit 0
fi

mkdir -p "${TARGET_BIN_DIR}"
mkdir -p "${INSTALL_DIR}"

TAG="$(resolve_release_tag)"
echo "Resolved llama.cpp release: ${TAG}"

SUCCESS=false

if [ "${USE_BREW}" = true ]; then
  if install_via_brew; then
    SUCCESS=true
  else
    echo "Error: Homebrew installation failed." >&2
    exit 1
  fi
elif [ "${MODE}" = "source" ]; then
  if build_from_source "${TAG}"; then
    SUCCESS=true
  else
    echo "Error: Building llama.cpp from source failed." >&2
    exit 1
  fi
elif [ "${MODE}" = "prebuilt" ]; then
  if install_prebuilt "${TAG}"; then
    SUCCESS=true
  else
    echo "Error: Prebuilt binary download failed." >&2
    exit 1
  fi
else
  # "auto" mode: Try prebuilt first, fall back to building from source
  if install_prebuilt "${TAG}"; then
    SUCCESS=true
  else
    echo "  Falling back to compiling llama.cpp from source..."
    if build_from_source "${TAG}"; then
      SUCCESS=true
    fi
  fi
fi

if [ "${SUCCESS}" = true ]; then
  echo
  echo "=================================================="
  echo "  ✓ llama.cpp / llama-server installation complete!"
  echo "=================================================="
  echo "Binaries located in : ${TARGET_BIN_DIR}"
  echo "Bundle directory    : ${INSTALL_DIR}"
  echo
  echo "Run '${TARGET_BIN_DIR}/llama-server --help' to inspect server flags."
  echo "Run './setup.sh --doctor' to verify your lokol environment."
  exit 0
else
  echo
  echo "❌ Installation failed. Please check error logs above." >&2
  exit 1
fi
