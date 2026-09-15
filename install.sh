#!/usr/bin/env bash
#
# teamlead-cli Installer Script
# Fast, conflict-free installer for macOS and Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/JordyDevrix/teamlead-cli/main/install.sh | bash
#   or:
#   ./install.sh [OPTIONS]
#
# Options:
#   -v, --version <version>    Specify version to install (e.g. v1.0.0, default: latest)
#   -d, --dir <path>           Installation directory (default: auto-detect clash-free dir)
#   -r, --repo <owner/repo>    GitHub repository (default: JordyDevrix/teamlead-cli)
#       --no-modify-path       Do not modify shell profile (.zshrc, .bashrc, etc.)
#   -f, --force                Force reinstall/overwrite existing binary
#   -h, --help                 Display this help message
#
# Environment variables:
#   VERSION, INSTALL_DIR, BIN_DIR, GITHUB_REPO, TEAMLEAD_REPO, NO_MODIFY_PATH, FORCE, NO_COLOR

set -euo pipefail

# --- Color and formatting support ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    BOLD="\033[1m"
    GREEN="\033[32m"
    CYAN="\033[36m"
    YELLOW="\033[33m"
    RED="\033[31m"
    RESET="\033[0m"
else
    BOLD=""
    GREEN=""
    CYAN=""
    YELLOW=""
    RED=""
    RESET=""
fi

log_info() {
    printf "${CYAN}[INFO]${RESET} %s\n" "$1"
}

log_success() {
    printf "${GREEN}[OK]${RESET} %b\n" "$1"
}

log_warn() {
    printf "${YELLOW}[WARN]${RESET} %s\n" "$1"
}

log_error() {
    printf "${RED}[ERROR]${RESET} %s\n" "$1" >&2
}

# --- Default configuration ---
DEFAULT_REPO="JordyDevrix/teamlead-cli"
REPO="${GITHUB_REPO:-${TEAMLEAD_REPO:-$DEFAULT_REPO}}"
TARGET_VERSION="${VERSION:-latest}"
CUSTOM_DIR="${INSTALL_DIR:-${BIN_DIR:-}}"
NO_MODIFY_PATH_FLAG="${NO_MODIFY_PATH:-0}"
FORCE_FLAG="${FORCE:-0}"
LOCAL_FILE=""
TMP_DIR=""

cleanup() {
    if [ -n "${TMP_DIR:-}" ] && [ -d "${TMP_DIR:-}" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT INT TERM

# --- Help message ---
show_help() {
    cat <<EOF
${BOLD}teamlead-cli Installer${RESET}
Installs teamlead standalone CLI binary cleanly and safely.

${BOLD}USAGE:${RESET}
  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash
  or:
  ./install.sh [OPTIONS]

${BOLD}OPTIONS:${RESET}
  -v, --version <version>    Version to install (e.g. v1.0.0, default: latest)
  -d, --dir <path>           Installation directory (default: auto-detected clash-free path)
  -r, --repo <owner/repo>    GitHub repository (default: ${DEFAULT_REPO})
      --no-modify-path       Skip shell profile modification (.zshrc, .bashrc, etc.)
  -f, --force                Overwrite existing binary without prompting
      --local <file>         Install from local archive or binary (for testing/air-gapped)
  -h, --help                 Show this help message

${BOLD}ENVIRONMENT VARIABLES:${RESET}
  VERSION, INSTALL_DIR, GITHUB_REPO, NO_MODIFY_PATH, FORCE, NO_COLOR
EOF
}

# --- Parse arguments ---
while [ $# -gt 0 ]; do
    case "$1" in
        -v|--version)
            TARGET_VERSION="$2"
            shift 2
            ;;
        -d|--dir)
            CUSTOM_DIR="$2"
            shift 2
            ;;
        -r|--repo)
            REPO="$2"
            shift 2
            ;;
        --no-modify-path)
            NO_MODIFY_PATH_FLAG="1"
            shift
            ;;
        -f|--force)
            FORCE_FLAG="1"
            shift
            ;;
        --local)
            LOCAL_FILE="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# --- Platform & architecture detection ---
detect_os() {
    local raw_os
    raw_os="$(uname -s)"
    case "$raw_os" in
        Darwin*) echo "darwin" ;;
        Linux*)  echo "linux" ;;
        MSYS*|MINGW*|CYGWIN*)
            log_error "Windows environment detected under shell ($raw_os)."
            log_error "Please run the PowerShell installer instead:"
            log_error "  irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
            exit 1
            ;;
        *)
            log_error "Unsupported operating system: $raw_os"
            exit 1
            ;;
    esac
}

detect_arch() {
    local raw_arch
    raw_arch="$(uname -m)"
    case "$raw_arch" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)
            log_error "Unsupported CPU architecture: $raw_arch"
            exit 1
            ;;
    esac
}

# --- HTTP Downloader helper ---
download_file() {
    local url="$1"
    local dest="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$dest"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$dest" "$url"
    else
        log_error "Neither curl nor wget was found. Please install curl or wget."
        exit 1
    fi
}

# --- Check if directory is in current $PATH ---
dir_in_path() {
    local check_dir="${1%/}"
    case ":$PATH:" in
        *":$check_dir:"*|*":$check_dir/:"*)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# --- Clash-free directory resolution ---
resolve_install_dir() {
    if [ -n "$CUSTOM_DIR" ]; then
        echo "$CUSTOM_DIR"
        return
    fi

    # Strategy: Prioritize locations that already exist in $PATH so no shell
    # configuration files or environment variables ever need to be modified.

    # 1. Check if /usr/local/bin is writable by current user
    #    (/usr/local/bin is almost universally in $PATH on macOS and Linux by default)
    if [ -w "/usr/local/bin" ] || [ "$(id -u 2>/dev/null || echo 1)" -eq 0 ]; then
        echo "/usr/local/bin"
        return
    fi

    # 2. Check XDG_BIN_HOME if set and present in PATH
    if [ -n "${XDG_BIN_HOME:-}" ] && dir_in_path "$XDG_BIN_HOME"; then
        echo "$XDG_BIN_HOME"
        return
    fi

    # 3. Check ~/.local/bin (standard modern user binary directory)
    if dir_in_path "$HOME/.local/bin"; then
        echo "$HOME/.local/bin"
        return
    fi

    # 4. Check ~/bin
    if dir_in_path "$HOME/bin"; then
        echo "$HOME/bin"
        return
    fi

    # 5. Default user destination: ~/.local/bin
    echo "$HOME/.local/bin"
}

# --- Detect active shell profile file ---
detect_shell_rc() {
    local shell_bin
    shell_bin="$(basename "${SHELL:-bash}")"

    case "$shell_bin" in
        zsh)
            echo "${ZDOTDIR:-$HOME}/.zshrc"
            ;;
        bash)
            # macOS uses .bash_profile for login shells; Linux commonly uses .bashrc
            if [ "$(uname -s)" = "Darwin" ]; then
                if [ -f "$HOME/.bash_profile" ]; then
                    echo "$HOME/.bash_profile"
                elif [ -f "$HOME/.bashrc" ]; then
                    echo "$HOME/.bashrc"
                else
                    echo "$HOME/.bash_profile"
                fi
            else
                if [ -f "$HOME/.bashrc" ]; then
                    echo "$HOME/.bashrc"
                else
                    echo "$HOME/.profile"
                fi
            fi
            ;;
        fish)
            echo "$HOME/.config/fish/config.fish"
            ;;
        *)
            if [ -f "$HOME/.profile" ]; then
                echo "$HOME/.profile"
            else
                echo "$HOME/.bashrc"
            fi
            ;;
    esac
}

# --- Safe, idempotent shell configuration ---
ensure_path_configured() {
    local target_dir="$1"

    # If the directory is already in $PATH, do not touch any profile!
    if dir_in_path "$target_dir"; then
        log_success "${BOLD}$target_dir${RESET} is already in your PATH. No profile modification needed."
        return 0
    fi

    if [ "$NO_MODIFY_PATH_FLAG" = "1" ] || [ "$NO_MODIFY_PATH_FLAG" = "true" ]; then
        log_info "Skipping profile update (--no-modify-path specified)."
        log_warn "To run teamlead from anywhere, add this directory to your PATH manually:"
        printf "    ${BOLD}export PATH=\"%s:\$PATH\"${RESET}\n\n" "$target_dir"
        return 0
    fi

    local rc_file
    rc_file="$(detect_shell_rc)"

    # Check if target_dir is already mentioned in the RC file
    if [ -f "$rc_file" ] && grep -Fq "$target_dir" "$rc_file" 2>/dev/null; then
        log_info "$target_dir is already configured in ${BOLD}$rc_file${RESET}."
        log_info "Restart your shell or run: ${BOLD}source $rc_file${RESET}"
        return 0
    fi

    # Append safely with clean, clear comment boundaries
    log_info "Adding $target_dir to PATH in ${BOLD}$rc_file${RESET}..."
    mkdir -p "$(dirname "$rc_file")"

    if [ "$(basename "${SHELL:-}")" = "fish" ]; then
        printf "\n# >>> teamlead PATH >>>\nfish_add_path -a \"%s\"\n# <<< teamlead PATH <<<\n" "$target_dir" >> "$rc_file"
    else
        printf "\n# >>> teamlead PATH >>>\nexport PATH=\"%s:\$PATH\"\n# <<< teamlead PATH <<<\n" "$target_dir" >> "$rc_file"
    fi

    log_success "Updated ${BOLD}$rc_file${RESET}."
    log_info "To use teamlead immediately in this session, run:"
    printf "    ${BOLD}source %s${RESET}\n\n" "$rc_file"
}

# --- Verification of checksum ---
verify_sha256() {
    local file="$1"
    local expected_hash="$2"

    if command -v sha256sum >/dev/null 2>&1; then
        local actual_hash
        actual_hash="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        local actual_hash
        actual_hash="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        log_warn "Neither sha256sum nor shasum is available. Skipping checksum verification."
        return 0
    fi

    if [ "$actual_hash" != "$expected_hash" ]; then
        log_error "SHA256 checksum mismatch!"
        log_error "Expected: $expected_hash"
        log_error "Actual:   $actual_hash"
        return 1
    fi

    log_success "SHA256 checksum verified (${actual_hash:0:12}...)"
    return 0
}

# --- Main installation procedure ---
main() {
    printf "\n${BOLD}${CYAN}🛡️  teamlead-cli Installer${RESET}\n"
    printf "   Conflict-free multi-agent coordinator for AI coding agents\n\n"

    local os arch install_dir
    os="$(detect_os)"
    arch="$(detect_arch)"
    install_dir="$(resolve_install_dir)"

    log_info "Detected system: ${BOLD}${os}-${arch}${RESET}"
    log_info "Install target:  ${BOLD}${install_dir}/teamlead${RESET}"

    # Setup temporary working directory
    TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'teamlead-install')"

    local binary_source=""

    if [ -n "$LOCAL_FILE" ]; then
        log_info "Using local source: $LOCAL_FILE"
        if [ -d "$LOCAL_FILE" ]; then
            if [ -f "$LOCAL_FILE/teamlead" ]; then
                binary_source="$LOCAL_FILE/teamlead"
            elif [ -f "$LOCAL_FILE/teamlead-cli" ]; then
                binary_source="$LOCAL_FILE/teamlead-cli"
            fi
        elif [ "${LOCAL_FILE##*.}" = "gz" ] || [ "${LOCAL_FILE##*.}" = "tgz" ]; then
            tar -xzf "$LOCAL_FILE" -C "$TMP_DIR"
            binary_source="$(find "$TMP_DIR" -type f \( -name "teamlead" -o -name "teamlead-cli" \) | head -n 1)"
        else
            binary_source="$LOCAL_FILE"
        fi
    else
        # Determine release archive name
        local archive_name="teamlead-${os}-${arch}.tar.gz"
        local base_url

        if [ "$TARGET_VERSION" = "latest" ]; then
            base_url="https://github.com/${REPO}/releases/latest/download"
        else
            case "$TARGET_VERSION" in
                v*) ;;
                *) TARGET_VERSION="v${TARGET_VERSION}" ;;
            esac
            base_url="https://github.com/${REPO}/releases/download/${TARGET_VERSION}"
        fi

        local download_url="${base_url}/${archive_name}"
        local checksum_url="${base_url}/checksums.txt"
        local archive_dest="${TMP_DIR}/${archive_name}"
        local checksum_dest="${TMP_DIR}/checksums.txt"

        log_info "Downloading ${archive_name} from ${download_url}..."
        download_file "$download_url" "$archive_dest"

        # Download checksums if available and verify
        if download_file "$checksum_url" "$checksum_dest" 2>/dev/null; then
            local expected_hash
            expected_hash="$(grep -F "$archive_name" "$checksum_dest" | awk '{print $1}' | head -n1 || true)"
            if [ -n "$expected_hash" ]; then
                verify_sha256 "$archive_dest" "$expected_hash"
            fi
        else
            log_warn "Checksums file could not be retrieved; proceeding with downloaded archive."
        fi

        log_info "Extracting archive..."
        tar -xzf "$archive_dest" -C "$TMP_DIR"

        if [ -f "$TMP_DIR/teamlead" ]; then
            binary_source="$TMP_DIR/teamlead"
        elif [ -f "$TMP_DIR/teamlead-cli" ]; then
            binary_source="$TMP_DIR/teamlead-cli"
        else
            binary_source="$(find "$TMP_DIR" -type f \( -name "teamlead" -o -name "teamlead-cli" \) | head -n 1)"
        fi
    fi

    if [ -z "$binary_source" ] || [ ! -f "$binary_source" ]; then
        log_error "Could not find teamlead binary in extracted package."
        exit 1
    fi

    # Make executable
    chmod +x "$binary_source"

    # Install into target directory
    mkdir -p "$install_dir"
    local target_binary="$install_dir/teamlead"
    local target_alias="$install_dir/teamlead-cli"

    if [ -f "$target_binary" ] && [ "$FORCE_FLAG" != "1" ] && [ "$FORCE_FLAG" != "true" ]; then
        log_info "Overwriting existing teamlead installation at $target_binary..."
    fi

    # Copy into target directory
    cp -f "$binary_source" "$target_binary"
    chmod 755 "$target_binary"

    # Also symlink teamlead-cli -> teamlead for convenience
    ln -sf "$target_binary" "$target_alias"

    # Verify installation
    if ! "$target_binary" --version >/dev/null 2>&1; then
        log_error "Installed binary failed smoke test ($target_binary --version)."
        exit 1
    fi

    local installed_version
    installed_version="$("$target_binary" --version 2>/dev/null || echo "teamlead")"

    log_success "${BOLD}Successfully installed ${installed_version}${RESET} to ${BOLD}${target_binary}${RESET}"

    # Handle PATH configuration without clashing
    ensure_path_configured "$install_dir"

    printf "${GREEN}${BOLD}✓ Installation complete!${RESET}\n\n"
    printf "To get started with teamlead:\n"
    printf "  1. Initialize in a git repository:  ${CYAN}teamlead init${RESET}\n"
    printf "  2. Add a scoped task:              ${CYAN}teamlead task add \"My Task\" --scope \"src/**\"${RESET}\n"
    printf "  3. Launch an agent safely:         ${CYAN}teamlead run --agent claude --task T-1 -- claude${RESET}\n"
    printf "  4. View live multi-agent board:    ${CYAN}teamlead status${RESET}\n\n"
}

main "$@"
