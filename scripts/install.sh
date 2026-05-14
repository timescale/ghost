#!/bin/sh
# shellcheck shell=dash
# shellcheck disable=SC2039 # local is non-POSIX
#
# Ghost Installation Script
#
# This script automatically downloads and installs the latest version of Ghost
# from the release server. It detects your platform (OS and architecture) and
# downloads the appropriate binary for your system.
#
# Usage:
#   curl -fsSL https://install.ghost.build | sh
#
# Environment Variables (all optional):
#   VERSION           - Specific version to install (e.g., "v1.2.3")
#                       Default: installs the latest version
#
#   INSTALL_DIR       - Custom installation directory
#                       Default: auto-detects best location
#
# Supported Platforms:
#   - Linux (x86_64, i386, arm64, armv7)
#   - macOS/Darwin (x86_64, arm64)
#   - Windows (x86_64)
#
# Requirements:
#   - curl (for downloading)
#   - tar/unzip (for extracting archives)
#   - shasum/sha256sum (for verifying checksums)
#   - Standard POSIX utilities (mktemp, chmod, etc.)
set -eu

# ============================================================================
# Configuration
# ============================================================================

REPO_NAME="ghost"
BINARY_NAME="ghost"

# Download URL
DOWNLOAD_BASE_URL="https://install.ghost.build"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# Shared utilities
# ============================================================================

# True when stderr is a TTY that supports ANSI escape sequences (cursor
# positioning, in-place updates, colors). False for pipes, redirected
# files, and dumb terminals — TERM=dumb is the historical Unix marker
# for environments (emacs shell-mode, some CI runners) that don't
# understand control sequences.
supports_ansi_escapes() {
    [ -t 2 ] && [ "${TERM:-}" != "dumb" ]
}

# Logging functions. log_info is gated on QUIET so the post-animation flow
# can suppress its own helpers' chatter while showing explicit status
# updates. log_warn and log_error clear any in-place status line first so
# their output doesn't get jumbled with whatever was there.
log_info() {
    if [ "${QUIET:-false}" = "true" ]; then
        return
    fi
    printf "%s\n" "$1" >&2
}

log_debug() {
    if [ "${QUIET:-false}" = "true" ]; then
        return
    fi
    if [ "${DEBUG:-false}" = "true" ]; then
      printf "%s\n" "$1" >&2
    fi
}

log_success() {
    printf "%s\n" "$1" >&2
}

log_warn() {
    if [ -t 2 ]; then
        printf "\r\033[K" >&2
    fi
    printf "%b[WARN]%b %s\n" "${YELLOW}" "${NC}" "$1" >&2
}

log_error() {
    if [ -t 2 ]; then
        printf "\r\033[K" >&2
    fi
    printf "%b[ERROR]%b %s\n" "${RED}" "${NC}" "$1" >&2
}

# Overwrite the current status line with the given content. There are
# three modes:
#   1. STATUS_FILE is set (animated mode with backgrounded animation):
#      write the rendered text to the file. The animation reads it each
#      frame, so the ghost stays "alive" while the status updates.
#   2. ANSI-capable TTY (no STATUS_FILE): write directly with \r\033[K so
#      successive calls overwrite the same line.
#   3. Non-TTY (pipe, dumb terminal): one fresh line per call.
update_status_line() {
    if [ -n "${STATUS_FILE:-}" ]; then
        printf "%b" "$1" > "${STATUS_FILE}"
        # Pause briefly so the background animation has time to read and
        # render this status before the next call potentially overwrites
        # it. Without this, fast-changing statuses (e.g. an extract step
        # that finishes in a few ms) can be replaced before any animation
        # poll observes them, so the user never sees them on screen.
        sleep 0.075
        return
    fi
    if supports_ansi_escapes; then
        printf "\r\033[K%b" "$1" >&2
    else
        printf "%b\n" "$1" >&2
    fi
}

# Read the first line of a file, stripping any trailing CR/LF. Returns
# empty when the file doesn't exist or is empty so callers can treat
# absent and empty files identically.
read_first_line() {
    [ -s "$1" ] || return 0
    head -n1 "$1" | tr -d '\n\r'
}

# Get the size of a file in bytes. Returns 0 if the file doesn't exist.
file_size() {
    if [ ! -f "$1" ]; then
        echo 0
        return
    fi
    wc -c < "$1" 2>/dev/null | tr -d ' \t' || echo 0
}

# Fetch the Content-Length of a URL via a HEAD request. Returns empty if
# the server doesn't expose Content-Length.
content_length() {
    local url="$1"
    curl -sLI "${url}" 2>/dev/null | tr -d '\r' | awk -F: 'tolower($1) == "content-length" { gsub(/[ \t]/, "", $2); print $2 }' | tail -n1
}

# ============================================================================
# Install flow helpers
# ============================================================================

# Detect OS and architecture
detect_platform() {
    local os
    local arch

    # Detect OS
    case "$(uname -s)" in
        Darwin*) os="darwin" ;;
        Linux*)  os="linux" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *) log_error "Unsupported operating system: $(uname -s)"; exit 1 ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64) arch="x86_64" ;;
        i386|i686) arch="i386" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l) arch="armv7" ;;
        *) log_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

# Verify that all required dependencies are available
verify_dependencies() {
    local platform="$1"

    # Build complete dependency list based on platform
    local required_deps="curl mktemp head tr sed awk grep uname chmod cp mkdir sleep cat wc"

    if echo "${platform}" | grep -q "windows"; then
        required_deps="${required_deps} unzip"
    else
        required_deps="${required_deps} tar"
    fi

    # Check if all commands are available
    local missing_deps=""
    local cmd

    for cmd in ${required_deps}; do
        if ! command -v "${cmd}" >/dev/null 2>&1; then
            missing_deps="${missing_deps} ${cmd}"
        fi
    done

    if [ -n "${missing_deps}" ]; then
        log_error "Missing required dependencies:${missing_deps}"
        log_error "Please install these tools and try again"
        exit 1
    fi
}

# Run curl with retries and exponential backoff. The action verb (e.g.
# "fetch", "download") and description appear in log messages; the
# remaining args are passed through to curl. Exits the script if all
# retries fail.
curl_with_retry() {
    local action="$1"
    local description="$2"
    local url="$3"
    shift 3
    local max_retries=3
    local retry_count=0
    local backoff_seconds=1

    while [ "${retry_count}" -le "${max_retries}" ]; do
        if curl -fsSL "$@" "${url}"; then
            return 0
        fi
        retry_count=$((retry_count + 1))
        if [ "${retry_count}" -le "${max_retries}" ]; then
            log_warn "${description} ${action} failed, retrying (${retry_count}/${max_retries})..."
            sleep "${backoff_seconds}"
            backoff_seconds=$((backoff_seconds * 2))
        else
            log_error "Failed to ${action} ${description} after $((max_retries + 1)) attempts"
            log_error "URL: ${url}"
            exit 1
        fi
    done
}

# Download a URL to stdout with retry logic
fetch_with_retry() {
    local url="$1"
    local description="${2:-content}"
    curl_with_retry fetch "${description}" "${url}"
}

# Download a file with retry logic
download_with_retry() {
    local url="$1"
    local output_file="$2"
    local description="${3:-file}"
    log_info "Downloading ${description}..."
    log_info "URL: ${url}"
    curl_with_retry download "${description}" "${url}" -o "${output_file}"
}

# Get version (from VERSION env var or latest from CloudFront)
get_version() {
    # Use VERSION env var if provided
    if [ -n "${VERSION:-}" ]; then
        log_info "Using specified version: ${VERSION}"
        echo "${VERSION}"
        return
    fi

    local url="${DOWNLOAD_BASE_URL}/latest.txt"

    # Try to get version from latest.txt file
    local version
    version=$(fetch_with_retry "${url}" "latest version")

    # Clean up the version string
    version=$(echo "${version}" | head -n1 | tr -d '\n\r')

    if [ -z "${version}" ]; then
        log_error "latest.txt file is empty"
        exit 1
    fi

    log_info "Latest version: ${version}"
    echo "${version}"
}

# Check if a directory is in PATH
is_in_path() {
    local dir="$1"
    case ":${PATH}:" in
        *":${dir}:"*) return 0 ;;
        *) return 1 ;;
    esac
}

# Ensure a directory exists and is writable, creating it if needed
ensure_writable_dir() {
    local dir="$1"

    if [ -d "${dir}" ] && [ -w "${dir}" ]; then
        return 0  # Directory exists and is writable
    elif [ ! -e "${dir}" ] && [ -w "$(dirname "${dir}")" ]; then
        # Directory doesn't exist but parent is writable - create it
        mkdir -p "${dir}"
        return 0
    else
        return 1  # Neither condition met
    fi
}

# Find the best install directory and ensure it exists
detect_install_dir() {
    # If user specified INSTALL_DIR, respect it and try to use it
    if [ -n "${INSTALL_DIR:-}" ]; then
        if ensure_writable_dir "${INSTALL_DIR}"; then
            log_info "Using user-specified install directory: ${INSTALL_DIR}"
            echo "${INSTALL_DIR}"
            return
        else
            log_error "User-specified install directory is not writable: ${INSTALL_DIR}"
            exit 1
        fi
    fi

    # Priority 1: Try to find a directory that's writable/creatable and in PATH
    for dir in "$HOME/.local/bin" "$HOME/bin"; do
        if ensure_writable_dir "${dir}" && is_in_path "${dir}"; then
            log_info "Selected install directory: ${dir}"
            echo "${dir}"
            return
        fi
    done

    # Priority 2: Try to find any directory that's writable/creatable (not in PATH)
    for dir in "$HOME/.local/bin" "$HOME/bin"; do
        if ensure_writable_dir "${dir}"; then
            log_info "Selected install directory: ${dir}"
            echo "${dir}"
            return
        fi
    done

    # No suitable directory found, fail with clear error
    log_error "Cannot find a writable install directory"
    log_error "Tried: \$HOME/.local/bin, \$HOME/bin"
    log_error "Please set INSTALL_DIR environment variable to a writable directory"
    exit 1
}


# Build archive name based on platform
build_archive_name() {
    local platform="$1"

    if [ "${platform}" = "windows_x86_64" ]; then
        echo "${REPO_NAME}_Windows_x86_64.zip"
    else
        echo "${REPO_NAME}_$(echo "${platform}" | sed 's/_/ /' | awk '{print toupper(substr($1,1,1)) tolower(substr($1,2)) "_" $2}').tar.gz"
    fi
}

# Download and validate checksum file
verify_checksum() {
    local version="$1"
    local filename="$2"
    local tmp_dir="$3"

    # Construct individual checksum file URL
    local checksum_url="${DOWNLOAD_BASE_URL}/releases/${version}/${filename}.sha256"
    local checksum_file="${tmp_dir}/${filename}.sha256"

    # Download checksum file with retry logic
    download_with_retry "${checksum_url}" "${checksum_file}" "checksum file"

    log_info "Validating checksum for ${filename}..."

    cd "${tmp_dir}"

    # Format checksum for validation: "hash  filename"
    local formatted_checksum
    formatted_checksum=$(printf "%s  %s\n" "$(tr -d '[:space:]' < "${checksum_file}")" "${filename}")

    if command -v sha256sum >/dev/null 2>&1; then
        if ! echo "${formatted_checksum}" | sha256sum -c - >/dev/null 2>&1; then
            log_error "Checksum validation failed using sha256sum"
            log_error "For security reasons, installation has been aborted"
            exit 1
        fi
    elif command -v shasum >/dev/null 2>&1; then
        if ! echo "${formatted_checksum}" | shasum -a 256 -c - >/dev/null 2>&1; then
            log_error "Checksum validation failed using shasum"
            log_error "For security reasons, installation has been aborted"
            exit 1
        fi
    else
        log_error "No SHA256 utility available (tried sha256sum, shasum)"
        log_error "Checksum validation is required for security"
        log_error "Please install sha256sum or shasum and try again"
        exit 1
    fi
}

# Fetch the version, build the archive name, and download the archive (no
# checksum verification — that runs in the foreground after the animation
# so we can show separate status updates for each step). Intended to run in
# the background while the intro animation plays.
download_archive_for_platform() {
    local platform="$1"
    local tmp_dir="$2"
    local version_file="$3"
    local archive_name_file="$4"

    local version
    version="$(get_version)"
    printf "%s\n" "${version}" > "${version_file}"

    local archive_name
    archive_name="$(build_archive_name "${platform}")"
    printf "%s\n" "${archive_name}" > "${archive_name_file}"

    local download_url="${DOWNLOAD_BASE_URL}/releases/${version}/${archive_name}"
    download_with_retry "${download_url}" "${tmp_dir}/${archive_name}" "Ghost ${version} for ${platform}"
}

# Extract archive and return path to binary
extract_archive() {
    local archive_name="$1"
    local tmp_dir="$2"
    local platform="$3"

    log_info "Extracting archive..."
    cd "${tmp_dir}"

    local binary_path
    if [ "${platform}" = "windows_x86_64" ]; then
        unzip -q "${archive_name}"
        binary_path="${tmp_dir}/${BINARY_NAME}.exe"
    else
        tar -xzf "${archive_name}"
        binary_path="${tmp_dir}/${BINARY_NAME}"
    fi

    # Verify binary exists
    if [ ! -f "${binary_path}" ]; then
        log_error "Binary not found in archive"
        exit 1
    fi

    # Make binary executable
    chmod +x "${binary_path}"

    echo "${binary_path}"
}

# Verify installation
verify_installation() {
    local install_dir="$1"
    local installed_binary="$2"
    local binary_path="${install_dir}/${installed_binary}"

    # First, check if binary exists at expected location
    if [ ! -f "${binary_path}" ]; then
        log_error "Installation verification failed: Binary not found at ${binary_path}"
        exit 1
    fi

    # Test that the binary is executable and get version
    local installed_version
    if installed_version=$("${binary_path}" version --bare --version-check=false 2>/dev/null | head -n1 || echo ""); then
        if [ -n "${installed_version}" ]; then
            log_debug "Ghost ${installed_version} installed successfully!"
        else
            log_success "Binary installed successfully at ${binary_path}"
        fi
    else
        log_error "Installation verification failed: Binary exists but is not executable"
        exit 1
    fi
}

# Run `ghost init` to drive the post-install configuration flow (PATH setup,
# login, MCP server installation, shell completions). We pass
# --skip-if-configured so re-runs of the installer don't re-prompt the user
# unnecessarily.
#
# `ghost init` needs an interactive TTY for its multi-select prompts. We
# redirect stdin/stdout/stderr through /dev/tty so the flow works under
# `curl | sh`, where the script's stdin is the pipe from curl, and so prompts
# remain visible even if the installer itself is redirected. If /dev/tty isn't
# readable and writable (e.g. in a container with no tty), we run the
# non-interactive PATH setup and tell the user to run the full interactive init
# flow manually.
run_ghost_init() {
    local binary_path="$1"
    if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
        "${binary_path}" --version-check=false init path || true
        printf "\nRun '%s init' to finish configuring Ghost.\n" "${binary_path}" >&2
        return 0
    fi
    "${binary_path}" --version-check=false init --skip-if-configured </dev/tty >/dev/tty 2>/dev/tty || true
}

# ============================================================================
# main
# ============================================================================

# Globals tracked by install_cleanup_on_exit. Promoted out of main() so
# the EXIT trap can read them: backgrounded children (the download and
# the animation) ignore SIGINT per POSIX (`&` in a non-interactive shell
# sets SIGINT/SIGQUIT to SIG_IGN), so on Ctrl+C they would otherwise
# keep running — the animation drawing forever and curl chewing
# bandwidth — until they finish on their own.
tmp_dir=""
download_pid=""
animation_pid=""

install_cleanup_on_exit() {
    # Disable `set -e` for the duration of this handler. wait on a child
    # killed by a signal returns 128+signum (e.g. 143 for SIGTERM), which
    # would otherwise abort the trap before the cursor is restored.
    set +e
    # After kill, wait on the pid (with stderr swallowed) so the shell
    # reaps the exit status itself instead of printing a job-termination
    # notification like "Terminated: 15" when the script unwinds.
    if [ -n "${animation_pid}" ]; then
        kill "${animation_pid}" 2>/dev/null
        wait "${animation_pid}" 2>/dev/null
    fi
    if [ -n "${download_pid}" ]; then
        kill "${download_pid}" 2>/dev/null
        wait "${download_pid}" 2>/dev/null
    fi
    if [ -n "${tmp_dir}" ]; then
        rm -rf "${tmp_dir}"
    fi
    # Restore the terminal cursor (the animation hides it during play).
    printf '\033[?25h' >&2
}

main() {
    # Detect platform and verify dependencies before starting the background
    # download so failures happen before the animation hides output.
    local platform
    platform=$(detect_platform)
    verify_dependencies "${platform}"

    # Create temporary directory and install the cleanup trap. tmp_dir,
    # download_pid, and animation_pid are file-scope globals so the
    # cleanup function can read them when the script aborts.
    tmp_dir="$(mktemp -d)"
    trap install_cleanup_on_exit EXIT

    local version_file="${tmp_dir}/version"
    local archive_name_file="${tmp_dir}/archive_name"
    local download_log="${tmp_dir}/download.log"

    # Suppress info-level chatter from helpers while the in-place status
    # display is active. Warnings and errors still surface via log_warn /
    # log_error (which clear the status line first).
    local QUIET=true

    # Start the binary download in the background. The background task
    # writes the version and archive name to files (so the animation can
    # render the header as soon as the version is known) and downloads
    # the archive. Checksum verification, extraction, and install run
    # later in the foreground while the animation keeps blinking.
    download_archive_for_platform "${platform}" "${tmp_dir}" "${version_file}" "${archive_name_file}" > "${download_log}" 2>&1 &
    download_pid=$!

    # In animated mode, background the animation so the ghost keeps
    # blinking through every install step. STATUS_FILE is the channel
    # used by update_status_line to publish phase strings ("Verifying
    # integrity...", "✓ Installed to ...") that the animation reads each
    # frame. stop_file is touched by main when the install is fully done
    # — the animation does one final render with the latest status, then
    # exits.
    #
    # In non-animated mode the animation just renders a static ghost in
    # the foreground; STATUS_FILE stays empty so update_status_line writes
    # to stderr the way it always did.
    local STATUS_FILE=""
    local stop_file=""
    if supports_ansi_escapes; then
        STATUS_FILE="${tmp_dir}/status"
        stop_file="${tmp_dir}/animation_done"
        play_ghost_intro_animation "${platform}" "${version_file}" "${archive_name_file}" "${tmp_dir}" "${stop_file}" &
        animation_pid=$!
    else
        play_ghost_intro_animation "${platform}" "${version_file}" "${archive_name_file}" "${tmp_dir}" ""
    fi

    # Wait for the version and archive name files to appear so the non-
    # TTY fallback can print the header below.
    while [ ! -s "${version_file}" ] || [ ! -s "${archive_name_file}" ]; do
        if ! kill -0 "${download_pid}" 2>/dev/null; then
            break
        fi
        sleep 0.05
    done

    local version archive_name
    version="$(read_first_line "${version_file}")"
    archive_name="$(read_first_line "${archive_name_file}")"

    local installed_binary="${BINARY_NAME}"
    if echo "${platform}" | grep -q "windows"; then
        installed_binary="${BINARY_NAME}.exe"
    fi

    # In non-TTY mode the animation just printed the static ghost — the
    # header and progress weren't drawn. Print them now so the non-
    # interactive path still shows what's happening.
    if ! supports_ansi_escapes; then
        if [ -n "${version}" ]; then
            printf "%s\n" "$(format_install_header "${version}" "${platform}")" >&2
        fi
        if kill -0 "${download_pid}" 2>/dev/null; then
            printf "Downloading...\n" >&2
        fi
    fi

    if ! wait "${download_pid}"; then
        if [ -n "${animation_pid}" ]; then
            update_status_line "${RED}✗${NC} Download failed"
            touch "${stop_file}" 2>/dev/null
            wait "${animation_pid}" 2>/dev/null
        fi
        printf "\n" >&2
        cat "${download_log}" >&2
        exit 1
    fi

    # Run the install steps. update_status_line routes through STATUS_FILE
    # in animated mode (animation reads + renders) or stderr otherwise.
    update_status_line "Verifying integrity..."
    verify_checksum "${version}" "${archive_name}" "${tmp_dir}"

    local install_dir
    install_dir="$(detect_install_dir)"

    update_status_line "Extracting archive..."
    local binary_path
    binary_path="$(extract_archive "${archive_name}" "${tmp_dir}" "${platform}")"

    update_status_line "Installing to ${install_dir}..."
    rm -f "${install_dir}/${installed_binary}"
    cp "${binary_path}" "${install_dir}/${installed_binary}"

    update_status_line "${GREEN}✓${NC} Installed to ${install_dir}/${installed_binary}"

    # Stop the background animation and wait for it to do its final
    # render. After this, the cursor is positioned on the status line.
    if [ -n "${animation_pid}" ]; then
        touch "${stop_file}"
        wait "${animation_pid}"
    fi

    # Add blank lines before the subsequent (non-in-place) sections.
    printf "\n\n" >&2

    # Restore log_info so the interactive shellrc helpers and the final
    # usage messages can speak normally.
    QUIET=false

    verify_installation "${install_dir}" "${installed_binary}"

    run_ghost_init "${install_dir}/${installed_binary}"
}

# ============================================================================
# Intro animation
#
# Draws an animated ghost with header + download progress while the binary
# downloads in the background. Falls back to a static, uncolored ghost when
# stderr isn't a TTY that supports ANSI escapes. Shell forward-references
# resolve at call time, so main() can call play_ghost_intro_animation even
# though it's defined below.
# ============================================================================

# Format the install header line "Ghost vX.Y.Z - platform" with color
# codes. Used for both the in-place animation header and the static
# fallback printed by main() when ANSI escapes aren't supported.
format_install_header() {
    local version="$1"
    local platform="$2"
    printf "%bGhost%b %b%s%b - %s" "${BLUE}" "${NC}" "${GREEN}" "${version}" "${NC}" "${platform}"
}

# Render a 24-cell progress bar with a trailing percentage label using
# Unicode block characters for the filled segment and light shade for the
# empty segment.
render_progress_bar() {
    local percent="${1:-0}"
    if [ "${percent}" -gt 100 ]; then percent=100; fi
    if [ "${percent}" -lt 0 ]; then percent=0; fi
    local width=24
    local filled=$((percent * width / 100))
    local bar=""
    local i=0
    while [ "${i}" -lt "${filled}" ]; do
        bar="${bar}█"
        i=$((i + 1))
    done
    while [ "${i}" -lt "${width}" ]; do
        bar="${bar}░"
        i=$((i + 1))
    done
    printf "%s %3d%%" "${bar}" "${percent}"
}

# Render a single Braille-char ghost row at the given indent. The indent may
# be negative, in which case the row is partially clipped at the left edge.
# Leading characters are trimmed via parameter expansion. `?` in a glob
# matches one character regardless of its UTF-8 byte width, so each `#?`
# strips exactly one Braille cell. `row_width` is the number of visible
# cells in the row (16 for the original ghost rows, 17 for the half-shifted
# bottom rows used during the sub-character slide).
ghost_intro_render_row() {
    local indent="$1"
    local color="$2"
    local row="$3"
    local clear_line="$4"
    local reset="$5"
    local row_width="$6"

    if [ "${indent}" -ge 0 ]; then
        printf "%s%*s%s%s%s\n" "${clear_line}" "${indent}" '' "${color}" "${row}" "${reset}" >&2
    elif [ $((indent + row_width)) -gt 0 ]; then
        local trim=$((-indent))
        local trimmed="${row}"
        while [ "${trim}" -gt 0 ]; do
            trimmed="${trimmed#?}"
            trim=$((trim - 1))
        done
        printf "%s%s%s%s\n" "${clear_line}" "${color}" "${trimmed}" "${reset}" >&2
    else
        printf "%s\n" "${clear_line}" >&2
    fi
}

# Render an eye row with body-colored outer chars and eye-colored middle
# chars at the given indent. Eye chars span positions 4-11 of the row in
# both the original (16-cell) and half-shifted (17-cell) variants. Each
# segment is trimmed independently when the row is partially clipped at the
# left edge, so the eye color is preserved during the slide-in.
ghost_intro_render_eye_row() {
    local indent="$1"
    local body_color="$2"
    local eye_color="$3"
    local pre_eye="$4"      # 4 chars
    local eye_chars="$5"    # 8 chars
    local post_eye="$6"     # 4 chars (phase 0) or 5 chars (phase 1)
    local clear_line="$7"
    local reset="$8"
    local row_width="$9"    # 16 or 17

    if [ "${indent}" -ge 0 ]; then
        printf "%s%*s%s%s%s%s%s%s%s\n" "${clear_line}" "${indent}" '' \
            "${body_color}" "${pre_eye}" \
            "${eye_color}" "${eye_chars}" \
            "${body_color}" "${post_eye}" "${reset}" >&2
        return
    fi

    if [ $((indent + row_width)) -le 0 ]; then
        printf "%s\n" "${clear_line}" >&2
        return
    fi

    local trim=$((-indent))
    local pre_visible=""
    local eyes_visible=""
    local post_visible=""
    local segment_trim trimmed

    if [ "${trim}" -lt 4 ]; then
        segment_trim="${trim}"
        trimmed="${pre_eye}"
        while [ "${segment_trim}" -gt 0 ]; do
            trimmed="${trimmed#?}"
            segment_trim=$((segment_trim - 1))
        done
        pre_visible="${trimmed}"
    fi

    if [ "${trim}" -lt 12 ]; then
        segment_trim=$((trim - 4))
        if [ "${segment_trim}" -lt 0 ]; then segment_trim=0; fi
        trimmed="${eye_chars}"
        while [ "${segment_trim}" -gt 0 ]; do
            trimmed="${trimmed#?}"
            segment_trim=$((segment_trim - 1))
        done
        eyes_visible="${trimmed}"
    fi

    segment_trim=$((trim - 12))
    if [ "${segment_trim}" -lt 0 ]; then segment_trim=0; fi
    trimmed="${post_eye}"
    while [ "${segment_trim}" -gt 0 ]; do
        trimmed="${trimmed#?}"
        segment_trim=$((segment_trim - 1))
    done
    post_visible="${trimmed}"

    local output="${clear_line}"
    if [ -n "${pre_visible}" ]; then
        output="${output}${body_color}${pre_visible}"
    fi
    if [ -n "${eyes_visible}" ]; then
        output="${output}${eye_color}${eyes_visible}"
    fi
    if [ -n "${post_visible}" ]; then
        output="${output}${body_color}${post_visible}"
    fi
    output="${output}${reset}"
    printf "%s\n" "${output}" >&2
}

draw_ghost_intro_frame() {
    local indent="$1"
    local tilt="$2"
    local eyes_state="$3"
    local phase="$4"
    local esc="$5"
    local header="${6:-}"
    local status="${7:-}"
    # Empty esc means the caller wants plain (uncolored, no clear-line)
    # output — used by the static fallback when ANSI escapes aren't
    # supported. In that mode all the styling vars stay empty and the
    # printf calls below render bare text.
    local reset=""
    local clear_line=""
    local body=""
    local eyes=""
    if [ -n "${esc}" ]; then
        reset="${esc}0m"
        clear_line="${esc}2K"
        body="${esc}38;2;232;242;255m"
        eyes="${esc}38;2;102;247;65m"
    fi

    # Body rows are offset by `tilt` to suggest momentum at speed/direction
    # changes. The offset increases linearly down the body, so the bottom
    # leans further than the middle, like a swinging pendulum.
    local mid_indent=$((indent + tilt / 2))
    local bottom_indent=$((indent + tilt))

    # Phase 0 uses the original 16-cell rows; phase 1 uses pre-computed
    # half-shifted 17-cell variants of every row, which when rendered at the
    # same integer indent appear visually offset by half a cell to the right.
    # Alternating phase across consecutive frames produces smooth half-cell
    # motion for the entire ghost (head, body, and tail in lockstep), so
    # nothing visually disconnects during the slide-in.
    local row1 row2 row3_pre row3_post row4_pre row4_post
    local row5 row6 row7 row8 row9 row10
    local row_w eyes_top eyes_bot
    if [ "${phase}" = "1" ]; then
        row1="⠀⠀⠀⢀⣠⠔⠛⠉⠉⠙⠓⠢⣄⠀⠀⠀⠀"
        row2="⠀⠀⢀⡼⠉⠀⠀⠀⠀⠀⠀⠀⠙⢦⠀⠀⠀"
        row3_pre="⠀⠀⢼⠀"
        row3_post="⠀⠈⡆⠀⠀"
        row4_pre="⠀⢰⡃⠀"
        row4_post="⠀⠀⢻⠀⠀"
        row5="⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⡇⠀"
        row6="⠀⡞⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀"
        row7="⠀⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢹⠀"
        row8="⢸⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡄"
        row9="⠘⡇⠀⢀⣀⠀⠀⠀⢀⠀⠀⠀⠀⠀⠀⢸⡇"
        row10="⠀⠳⠴⠚⠙⠢⠤⠖⠚⠦⣤⠴⠒⠲⣄⡼⠁"
        row_w=17
        if [ "${eyes_state}" = "blink" ]; then
            eyes_top="⠀⠀⠀⠀⠀⠀⠀⠀"
            eyes_bot="⠀⠒⠒⠒⠀⠒⠒⠒"
        else
            eyes_top="⢠⣴⣦⣄⠀⣴⣶⣤"
            eyes_bot="⠸⣿⣿⠟⠈⢿⣿⣿"
        fi
    else
        row1="⠀⠀⠀⣀⡤⠚⠋⠉⠉⠛⠒⢤⡀⠀⠀⠀"
        row2="⠀⠀⣠⠏⠁⠀⠀⠀⠀⠀⠀⠈⠳⡄⠀⠀"
        row3_pre="⠀⢠⠇⠀"
        row3_post="⠀⢱⠀⠀"
        row4_pre="⠀⣞⠀⠀"
        row4_post="⠀⠘⡇⠀"
        row5="⠀⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢹⠀"
        row6="⢰⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⠀"
        row7="⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⡇"
        row8="⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣧"
        row9="⢻⠀⠀⣀⡀⠀⠀⠀⡀⠀⠀⠀⠀⠀⠀⣿"
        row10="⠘⠦⠖⠋⠓⠤⠴⠒⠳⢤⡤⠖⠒⢦⣠⠏"
        row_w=16
        if [ "${eyes_state}" = "blink" ]; then
            eyes_top="⠀⠀⠀⠀⠀⠀⠀⠀"
            eyes_bot="⠐⠒⠒⠂⠐⠒⠒⠂"
        else
            eyes_top="⣤⣶⣤⡀⢠⣶⣦⡄"
            eyes_bot="⢿⣿⡿⠃⠹⣿⣿⠇"
        fi
    fi

    # Padding above the ghost art.
    printf "%s\n" "${clear_line}" >&2

    # Head/upper rows (no tilt).
    ghost_intro_render_row "${indent}" "${body}" "${row1}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_row "${indent}" "${body}" "${row2}" "${clear_line}" "${reset}" "${row_w}"

    # Eye rows with segmented coloring (eyes always green, even partially).
    ghost_intro_render_eye_row "${indent}" "${body}" "${eyes}" "${row3_pre}" "${eyes_top}" "${row3_post}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_eye_row "${indent}" "${body}" "${eyes}" "${row4_pre}" "${eyes_bot}" "${row4_post}" "${clear_line}" "${reset}" "${row_w}"

    # Mid body rows, half tilt. Row 7 (the last mid row) is the boundary
    # between mid and bottom; when the body leans (bottom_indent !=
    # mid_indent) we render it at bottom_indent so its outline visually
    # connects to row 8 instead of leaving a horizontal step on the
    # leaning side. The body still reads as tilted because rows 5–6 stay
    # at mid_indent.
    local row7_indent="${mid_indent}"
    if [ "${bottom_indent}" -ne "${mid_indent}" ]; then
        row7_indent="${bottom_indent}"
    fi
    ghost_intro_render_row "${mid_indent}" "${body}" "${row5}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_row "${mid_indent}" "${body}" "${row6}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_row "${row7_indent}" "${body}" "${row7}" "${clear_line}" "${reset}" "${row_w}"

    # Bottom body rows, full tilt.
    ghost_intro_render_row "${bottom_indent}" "${body}" "${row8}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_row "${bottom_indent}" "${body}" "${row9}" "${clear_line}" "${reset}" "${row_w}"
    ghost_intro_render_row "${bottom_indent}" "${body}" "${row10}" "${clear_line}" "${reset}" "${row_w}"

    # Padding below the ghost art (separates the image from the header).
    printf "%s\n" "${clear_line}" >&2

    # Header + status lines (version/platform + download progress) anchor
    # at column 0, regardless of the ghost's animation indent. Skipped in
    # static (no-escape) mode since the caller prints those lines itself
    # and we don't need empty placeholders for cursor_up positioning.
    if [ -n "${esc}" ]; then
        printf "%s%s\n" "${clear_line}" "${header}" >&2
        printf "%s%s\n" "${clear_line}" "${status}" >&2
    fi
}

# Refresh the install_* state and rebuild the header/status display
# strings. Uses the calling function's locals (install_version, etc.) via
# shell's dynamic scoping. Sets header_buf and status_buf, which are read
# by the animation and post-animation blink loops.
ghost_intro_compute_display_state() {
    if [ -z "${install_version}" ] \
            && [ -s "${version_file:-/dev/null}" ] \
            && [ -s "${archive_name_file:-/dev/null}" ]; then
        install_version="$(read_first_line "${version_file}")"
        install_archive_name="$(read_first_line "${archive_name_file}")"
        install_archive_path="${tmp_dir}/${install_archive_name}"
        install_archive_url="${DOWNLOAD_BASE_URL}/releases/${install_version}/${install_archive_name}"
        install_total_bytes="$(content_length "${install_archive_url}")"
        : "${install_total_bytes:=0}"
    fi

    header_buf=""
    status_buf=""
    if [ -n "${install_version}" ]; then
        header_buf="$(format_install_header "${install_version}" "${platform}")"

        # Status priority: an explicit override from STATUS_FILE (set by
        # main() for post-download phases like "Verifying integrity...")
        # wins over the live download progress bar.
        if [ -n "${STATUS_FILE:-}" ] && [ -s "${STATUS_FILE}" ]; then
            status_buf="$(read_first_line "${STATUS_FILE}")"
        else
            local current_bytes=0
            if [ -n "${install_archive_path}" ]; then
                current_bytes="$(file_size "${install_archive_path}")"
            fi
            local percent=0
            if [ "${install_total_bytes}" -gt 0 ]; then
                percent=$((current_bytes * 100 / install_total_bytes))
                if [ "${percent}" -gt 100 ]; then percent=100; fi
            fi
            status_buf="Downloading $(render_progress_bar "${percent}")"
        fi
    fi
}

play_ghost_intro_animation() {
    local platform="${1:-}"
    local version_file="${2:-}"
    local archive_name_file="${3:-}"
    local tmp_dir="${4:-}"
    local stop_file="${5:-}"

    if ! supports_ansi_escapes; then
        # Static fallback: render the upright ghost with no escape codes.
        # The caller (main) handles the version header and progress lines
        # for the non-animated path.
        draw_ghost_intro_frame 2 0 open 0 ""
        return
    fi

    local esc
    local hide_cursor
    local show_cursor
    local cursor_up
    local frame
    local indent
    local eyes_state

    esc="$(printf '\033[')"
    hide_cursor="${esc}?25l"
    show_cursor="${esc}?25h"
    cursor_up="${esc}14A"

    printf "%s" "${hide_cursor}" >&2

    # Reserve 14 lines for the animation (1 padding + 10 ghost rows +
    # 1 padding + 1 header + 1 status), then move the cursor back to the
    # top of the reserved area. Without this, the very first frame has to
    # both push fresh lines onto the terminal (potentially scrolling the
    # viewport) and render content, which on some terminals stalls long
    # enough that the first frame appears static for a noticeable moment.
    # All subsequent frames just rewrite existing lines via cursor_up, so
    # pre-allocating here makes the first frame as fast as the rest.
    printf '\n\n\n\n\n\n\n\n\n\n\n\n\n\n' >&2
    printf "%s" "${cursor_up}" >&2

    # Frame data: "indent:tilt:eyes:phase". During the slide-in, alternating
    # phase=0 / phase=1 frames swap the bottom rows between their original
    # and half-shifted variants, giving the ghost's tail a 0.5-column step
    # rate while the head still moves in 1-column steps. After the slide,
    # all frames use phase=0 (integer cells). The final frame is `2:0:...:0`
    # so the ghost ends fully on-screen, upright, with 2 chars of left pad.
    local frames="\
-10:-1:open:0 -10:-1:open:1 -9:-1:open:0 -9:-1:open:1 -8:-1:open:0 -8:-1:open:1 \
-7:-1:open:0 -7:-1:open:1 -6:-1:open:0 -6:-1:open:1 -5:-1:open:0 -5:-1:open:1 \
-4:-1:open:0 -4:-1:open:1 -3:-1:open:0 -3:-1:open:1 -2:-1:open:0 -2:-1:open:1 \
-1:-1:open:0 -1:-1:open:1 0:-1:open:0 0:-1:open:1 1:-1:open:0 1:-1:open:1 \
2:-1:open:0 \
2:0:open:0 2:1:open:0 2:0:open:0 \
3:-1:open:0 4:-1:open:0 5:-1:open:0 6:-1:blink:0 7:-1:open:0 8:-1:open:0 9:-1:open:0 10:-1:open:0 \
10:0:open:0 10:1:open:0 \
9:1:open:0 8:1:open:0 7:1:blink:0 6:1:open:0 5:1:open:0 4:1:open:0 3:1:open:0 2:1:open:0 \
2:0:open:0 2:-1:open:0 2:0:open:0"

    local frame_data
    local tilt
    local phase
    local rest

    # Install state monitored each frame. The version + archive name files
    # are written by the background download task; once both exist we can
    # render the header and start showing real download progress.
    local install_version=""
    local install_archive_name=""
    local install_archive_path=""
    local install_archive_url=""
    local install_total_bytes=0
    local header_buf=""
    local status_buf=""

    frame=0
    for frame_data in $frames; do
        if [ "${frame}" -gt 0 ]; then
            printf "%s" "${cursor_up}" >&2
        fi

        ghost_intro_compute_display_state

        indent="${frame_data%%:*}"
        rest="${frame_data#*:}"
        tilt="${rest%%:*}"
        rest="${rest#*:}"
        eyes_state="${rest%%:*}"
        phase="${rest#*:}"

        draw_ghost_intro_frame "${indent}" "${tilt}" "${eyes_state}" "${phase}" "${esc}" "${header_buf}" "${status_buf}"
        frame=$((frame + 1))
        sleep 0.04
    done

    # After the main animation finishes, keep the ghost "alive" by
    # blinking its eyes intermittently. The loop runs until stop_file
    # appears — main touches it after the final install step — so the
    # ghost stays animated through verify, extract, and install.
    #
    # Polling at 50ms keeps reaction time tight enough that fast status
    # transitions are caught (combined with a small sleep in
    # update_status_line). To keep the cost low, we only repaint when
    # status_buf or eyes_state actually changes — most polls are no-ops.
    if [ -n "${stop_file}" ]; then
        local blink_frame=0
        local blink_cycle
        local last_status_rendered=""
        local last_eyes_rendered=""
        while [ ! -f "${stop_file}" ]; do
            ghost_intro_compute_display_state

            # Brief blink (~200ms) every ~2s. 40 frames * 50ms = 2s cycle;
            # last 4 frames (200ms) render with eyes blinking.
            blink_cycle=$((blink_frame % 40))
            eyes_state="open"
            if [ "${blink_cycle}" -ge 36 ]; then
                eyes_state="blink"
            fi

            if [ "${status_buf}" != "${last_status_rendered}" ] \
                    || [ "${eyes_state}" != "${last_eyes_rendered}" ]; then
                printf "%s" "${cursor_up}" >&2
                draw_ghost_intro_frame 2 0 "${eyes_state}" 0 "${esc}" "${header_buf}" "${status_buf}"
                last_status_rendered="${status_buf}"
                last_eyes_rendered="${eyes_state}"
            fi

            blink_frame=$((blink_frame + 1))
            sleep 0.05
        done

        # Final render with eyes open and the latest status (the caller
        # may have just written "✓ Installed to ..." to STATUS_FILE before
        # touching stop_file).
        printf "%s" "${cursor_up}" >&2
        ghost_intro_compute_display_state
        draw_ghost_intro_frame 2 0 open 0 "${esc}" "${header_buf}" "${status_buf}"
    fi

    # Show the cursor and position it on the status line (one row up from
    # the line below the last status print) so the caller can continue
    # updating the status in place via update_status_line.
    printf "%s%s" "${show_cursor}" "${esc}1A" >&2
}

# ============================================================================
# Entry point
# ============================================================================

main "$@"
