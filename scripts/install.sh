#!/usr/bin/env bash
# install.sh — build Fine Print for the current host and install it as a
# service. Runs bootstrap.sh first to fail fast on missing deps, then
# delegates the service-install step to install-service.sh.
#
# Usage: sudo make install
#   or:  sudo ./scripts/install.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

# --- Pre-flight ---------------------------------------------------------------

echo "→ Running dependency check"
bash "$SCRIPT_DIR/bootstrap.sh" || {
    echo "Dependency check failed. Fix the issues above and re-run."
    exit 1
}
echo

# --- Build --------------------------------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"

echo "→ Building frontend"
# `npm install` under sudo can create root-owned node_modules, making later
# dev work painful. Install as the invoking user when possible.
if [ -n "${SUDO_USER:-}" ] && [ "$(id -u)" = 0 ]; then
    sudo -u "$SUDO_USER" -H bash -c "cd '$REPO_ROOT/web' && npm install && npm run build"
else
    ( cd web && npm install && npm run build )
fi
rm -rf internal/frontend/build
cp -r web/build internal/frontend/build

echo "→ Building backend for $OS/$ARCH"
# Native build — install.sh runs on the target machine.
if [ -n "${SUDO_USER:-}" ] && [ "$(id -u)" = 0 ]; then
    sudo -u "$SUDO_USER" -H go build -o bin/fine-print ./cmd/fine-print
else
    go build -o bin/fine-print ./cmd/fine-print
fi

if [ ! -x bin/fine-print ]; then
    echo "Build did not produce bin/fine-print" >&2
    exit 1
fi

echo

# --- Install as service -------------------------------------------------------

if [ "$(id -u)" -ne 0 ]; then
    echo "Install step needs root. Re-run with: sudo make install"
    exit 1
fi

echo "→ Installing service"
bash "$SCRIPT_DIR/install-service.sh"
