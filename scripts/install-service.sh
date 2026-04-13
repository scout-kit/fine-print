#!/bin/bash
set -e

BINARY_NAME="fine-print"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/fine-print"
DATA_DIR="/var/lib/fine-print"
LOG_DIR="/var/log/fine-print"

echo "Fine Print - Service Installer"
echo "==============================="

# Check root
if [ "$(id -u)" -ne 0 ]; then
    echo "Error: This script must be run as root (sudo)"
    exit 1
fi

# Detect OS
OS="$(uname -s)"
ARCH="$(uname -m)"

echo "Detected: $OS $ARCH"

# Find binary
BINARY=""
if [ -f "./bin/$BINARY_NAME" ]; then
    BINARY="./bin/$BINARY_NAME"
elif [ -f "./$BINARY_NAME" ]; then
    BINARY="./$BINARY_NAME"
else
    echo "Error: Cannot find $BINARY_NAME binary."
    echo "Build it first with: make build"
    exit 1
fi

echo "Using binary: $BINARY"

# Create directories
echo "Creating directories..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"
mkdir -p "$LOG_DIR"

# Copy binary
echo "Installing binary to $INSTALL_DIR/$BINARY_NAME..."
cp "$BINARY" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Copy config if not exists
if [ ! -f "$CONFIG_DIR/config.yml" ]; then
    echo "Installing default config..."
    cp configs/fine-print.example.yml "$CONFIG_DIR/config.yml"
    # Update data_dir to production path
    sed -i.bak "s|data_dir: \"./data\"|data_dir: \"$DATA_DIR\"|" "$CONFIG_DIR/config.yml" 2>/dev/null || \
    sed -i '' "s|data_dir: \"./data\"|data_dir: \"$DATA_DIR\"|" "$CONFIG_DIR/config.yml"
    rm -f "$CONFIG_DIR/config.yml.bak"
    echo "  Edit $CONFIG_DIR/config.yml to set your admin password and preferences."
else
    echo "Config already exists at $CONFIG_DIR/config.yml (not overwritten)"
fi

# Install service
case "$OS" in
    Darwin)
        PLIST_SRC="configs/com.fineprint.app.plist"
        PLIST_DST="/Library/LaunchDaemons/com.fineprint.app.plist"

        echo "Installing launchd service..."
        cp "$PLIST_SRC" "$PLIST_DST"
        chmod 644 "$PLIST_DST"

        echo "Loading service..."
        launchctl load -w "$PLIST_DST" 2>/dev/null || true

        echo ""
        echo "Service installed. Commands:"
        echo "  Start:   sudo launchctl load -w $PLIST_DST"
        echo "  Stop:    sudo launchctl unload $PLIST_DST"
        echo "  Logs:    tail -f $LOG_DIR/stdout.log"
        ;;

    Linux)
        SERVICE_SRC="configs/fine-print.service"
        SERVICE_DST="/etc/systemd/system/fine-print.service"

        echo "Installing systemd service..."
        cp "$SERVICE_SRC" "$SERVICE_DST"
        chmod 644 "$SERVICE_DST"

        echo "Enabling and starting service..."
        systemctl daemon-reload
        systemctl enable fine-print
        systemctl start fine-print

        echo ""
        echo "Service installed and started. Commands:"
        echo "  Status:  sudo systemctl status fine-print"
        echo "  Stop:    sudo systemctl stop fine-print"
        echo "  Restart: sudo systemctl restart fine-print"
        echo "  Logs:    sudo journalctl -u fine-print -f"
        ;;

    *)
        echo "Warning: Unsupported OS for service installation."
        echo "Run manually: $INSTALL_DIR/$BINARY_NAME -config $CONFIG_DIR/config.yml"
        ;;
esac

echo ""
echo "Installation complete!"
echo ""
echo "Data directory:  $DATA_DIR"
echo "Config file:     $CONFIG_DIR/config.yml"
echo "Binary:          $INSTALL_DIR/$BINARY_NAME"
