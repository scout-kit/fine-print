#!/usr/bin/env bash
# bootstrap.sh — verify Fine Print's build + runtime prerequisites.
#
# Checks for Go, Node.js/npm, CUPS, and (on Linux) hostapd + dnsmasq.
# On failure it prints a copy-paste install command for the detected OS
# and exits non-zero. Safe to re-run: a fully-provisioned host reports OK
# and exits 0 without side effects.

set -euo pipefail

# --- Version floors -----------------------------------------------------------

MIN_GO_MAJOR=1
MIN_GO_MINOR=22
MIN_NODE_MAJOR=20

# --- OS detection -------------------------------------------------------------

OS_KIND=""         # "macos" | "debian" | "unknown"
OS_PRETTY=""
PKG_MANAGER=""
INSTALL_HINT=""

detect_os() {
    local uname_s
    uname_s="$(uname -s)"
    case "$uname_s" in
        Darwin)
            OS_KIND="macos"
            OS_PRETTY="macOS $(sw_vers -productVersion 2>/dev/null || echo '?')"
            PKG_MANAGER="brew"
            return
            ;;
        Linux)
            ;;
        *)
            OS_KIND="unknown"
            OS_PRETTY="$uname_s"
            return
            ;;
    esac

    if [ -r /etc/os-release ]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        OS_PRETTY="${PRETTY_NAME:-Linux}"
        case "${ID:-}:${ID_LIKE:-}" in
            debian*|ubuntu*|raspbian*|*:*debian*|*:*ubuntu*)
                OS_KIND="debian"
                PKG_MANAGER="apt-get"
                return
                ;;
        esac
    else
        OS_PRETTY="Linux (unknown distro)"
    fi
    OS_KIND="unknown"
}

# --- Helpers ------------------------------------------------------------------

has_cmd() { command -v "$1" >/dev/null 2>&1; }

# Compare semantic versions: returns 0 if $1 >= $2, non-zero otherwise.
version_ge() {
    # Expects dotted numeric versions like 1.22.3 and 1.22
    printf '%s\n%s\n' "$2" "$1" | sort -V -C 2>/dev/null && return 0 || return 1
}

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

MISSING=()
note_missing() { MISSING+=("$1"); }

# --- Individual checks --------------------------------------------------------

check_go() {
    if ! has_cmd go; then
        note_missing "go (>= ${MIN_GO_MAJOR}.${MIN_GO_MINOR})"
        return
    fi
    local ver
    ver="$(go version | awk '{print $3}' | sed 's/^go//')"
    if ! version_ge "$ver" "${MIN_GO_MAJOR}.${MIN_GO_MINOR}"; then
        note_missing "go (>= ${MIN_GO_MAJOR}.${MIN_GO_MINOR}, found $ver)"
        return
    fi
    green "✓ go $ver"
}

check_node() {
    if ! has_cmd node; then
        note_missing "node (>= ${MIN_NODE_MAJOR})"
        return
    fi
    local ver major
    ver="$(node --version | sed 's/^v//')"
    major="${ver%%.*}"
    if [ "$major" -lt "$MIN_NODE_MAJOR" ]; then
        note_missing "node (>= ${MIN_NODE_MAJOR}, found $ver)"
        return
    fi
    green "✓ node $ver"
    if ! has_cmd npm; then
        note_missing "npm (ships with node)"
    else
        green "✓ npm $(npm --version)"
    fi
}

check_cups() {
    # CUPS is required for printing. `lpstat` ships with cups on both macOS
    # and Debian. On macOS it's preinstalled; on Debian the `cups` package
    # provides it.
    if has_cmd lpstat; then
        green "✓ cups (lpstat present)"
    else
        note_missing "cups"
    fi
}

check_linux_hotspot() {
    [ "$OS_KIND" = "debian" ] || return 0
    local ok=1
    if ! has_cmd hostapd; then
        note_missing "hostapd"
        ok=0
    fi
    if ! has_cmd dnsmasq; then
        note_missing "dnsmasq"
        ok=0
    fi
    if [ "$ok" = 1 ]; then
        green "✓ hostapd + dnsmasq"
    fi
}

# --- Install hint generation --------------------------------------------------

print_install_hint() {
    [ "${#MISSING[@]}" -eq 0 ] && return 0

    red "Missing: ${MISSING[*]}"
    echo

    case "$OS_KIND" in
        macos)
            if ! has_cmd brew; then
                yellow "Install Homebrew first:  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
                echo
            fi
            local pkgs=()
            for m in "${MISSING[@]}"; do
                case "$m" in
                    go*)   pkgs+=("go") ;;
                    node*) pkgs+=("node") ;;
                    npm*)  ;;
                    cups*) pkgs+=("cups") ;;
                esac
            done
            if [ "${#pkgs[@]}" -gt 0 ]; then
                yellow "Install the missing packages:"
                echo "  brew install ${pkgs[*]}"
            fi
            ;;

        debian)
            local pkgs=()
            for m in "${MISSING[@]}"; do
                case "$m" in
                    go*)       pkgs+=("golang-go") ;;
                    node*)     pkgs+=("nodejs" "npm") ;;
                    npm*)      pkgs+=("npm") ;;
                    cups*)     pkgs+=("cups" "cups-client") ;;
                    hostapd*)  pkgs+=("hostapd") ;;
                    dnsmasq*)  pkgs+=("dnsmasq") ;;
                esac
            done
            if [ "${#pkgs[@]}" -gt 0 ]; then
                yellow "Install the missing packages:"
                echo "  sudo apt-get update && sudo apt-get install -y ${pkgs[*]}"
                echo
                yellow "Note: Debian's default Go/Node packages may be too old on older releases."
                echo "      For Go:    https://go.dev/dl/"
                echo "      For Node:  https://nodejs.org/en/download/package-manager (NodeSource repo)"
            fi
            ;;

        *)
            yellow "Unsupported OS for auto-hinting. Please install: ${MISSING[*]}"
            ;;
    esac

    echo
    red "After installing, re-run: make deps"
    return 1
}

# --- Main ---------------------------------------------------------------------

main() {
    detect_os
    echo "Fine Print — dependency check"
    echo "OS: $OS_PRETTY"
    echo

    check_go
    check_node
    check_cups
    check_linux_hotspot

    echo
    if [ "${#MISSING[@]}" -eq 0 ]; then
        green "All dependencies present. Run 'make install' to build and install the service."
        return 0
    fi

    print_install_hint
}

main "$@"
