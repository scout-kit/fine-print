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

# --- Require root upfront -----------------------------------------------------
# Service install needs root, and trying to drop privileges back to SUDO_USER
# fails when the repo lives in a root-owned tree like /opt. Just run the whole
# pipeline as root — the only side effect is root-owned node_modules and bin/,
# which is fine for a kiosk-style system install.

if [ "$(id -u)" -ne 0 ]; then
    echo "make install must be run with sudo:" >&2
    echo "  sudo make install" >&2
    exit 1
fi

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
( cd web && npm install && npm run build )
rm -rf internal/frontend/build
cp -r web/build internal/frontend/build

echo "→ Building backend for $OS/$ARCH"
go build -o bin/fine-print ./cmd/fine-print

if [ ! -x bin/fine-print ]; then
    echo "Build did not produce bin/fine-print" >&2
    exit 1
fi

echo

# --- Install as service -------------------------------------------------------

echo "→ Installing service"
bash "$SCRIPT_DIR/install-service.sh"
