#!/bin/bash
# ResourceAgent Linux Installation Script
# Run as root or with sudo

set -e

INSTALL_PATH="/opt/resourceagent"
CONFIG_PATH=""
SERVICE_USER="resourceagent"
UNINSTALL=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --install-path)
            INSTALL_PATH="$2"
            shift 2
            ;;
        --config)
            CONFIG_PATH="$2"
            shift 2
            ;;
        --uninstall)
            UNINSTALL=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

install_agent() {
    echo "Installing ResourceAgent..."

    # Create service user if not exists
    if ! id "$SERVICE_USER" &>/dev/null; then
        useradd --system --no-create-home --shell /bin/false "$SERVICE_USER"
        echo "Created service user: $SERVICE_USER"
    fi

    # Create installation directory
    mkdir -p "$INSTALL_PATH"/{configs,logs}
    echo "Created installation directory: $INSTALL_PATH"

    # Copy binary
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    BINARY_SOURCE="$SCRIPT_DIR/../resourceagent"

    if [[ ! -f "$BINARY_SOURCE" ]]; then
        BINARY_SOURCE="./resourceagent"
    fi

    if [[ -f "$BINARY_SOURCE" ]]; then
        cp "$BINARY_SOURCE" "$INSTALL_PATH/"
        chmod +x "$INSTALL_PATH/resourceagent"
        echo "Copied binary to $INSTALL_PATH"
    else
        echo "Error: Binary not found. Please build the project first."
        exit 1
    fi

    # Copy or create config
    if [[ -n "$CONFIG_PATH" && -f "$CONFIG_PATH" ]]; then
        cp "$CONFIG_PATH" "$INSTALL_PATH/configs/config.json"
        echo "Copied configuration from $CONFIG_PATH"
    else
        DEFAULT_CONFIG="$SCRIPT_DIR/../configs/config.json"
        if [[ -f "$DEFAULT_CONFIG" ]]; then
            cp "$DEFAULT_CONFIG" "$INSTALL_PATH/configs/config.json"
            echo "Copied default configuration"
        fi
    fi

    # Set permissions
    chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_PATH"

    # Install systemd service
    cat > /etc/systemd/system/resourceagent.service << EOF
[Unit]
Description=ResourceAgent Monitoring Service
Documentation=https://github.com/your-org/resourceagent
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$INSTALL_PATH/resourceagent -config $INSTALL_PATH/configs/config.json
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_PATH/logs
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    echo "Installed systemd service"

    # Reload systemd and start service
    systemctl daemon-reload
    systemctl enable resourceagent
    systemctl start resourceagent

    # Check status
    if systemctl is-active --quiet resourceagent; then
        echo "ResourceAgent installed and running successfully!"
    else
        echo "Warning: Service installed but not running. Check logs with: journalctl -u resourceagent"
    fi
}

uninstall_agent() {
    echo "Uninstalling ResourceAgent..."

    # Stop and disable service
    if systemctl is-active --quiet resourceagent; then
        systemctl stop resourceagent
    fi

    if systemctl is-enabled --quiet resourceagent 2>/dev/null; then
        systemctl disable resourceagent
    fi

    # Remove service file
    rm -f /etc/systemd/system/resourceagent.service
    systemctl daemon-reload
    echo "Removed systemd service"

    # Remove installation directory
    read -p "Remove installation directory $INSTALL_PATH? (y/N) " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        rm -rf "$INSTALL_PATH"
        echo "Removed installation directory"
    fi

    # Remove service user
    read -p "Remove service user $SERVICE_USER? (y/N) " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        userdel "$SERVICE_USER" 2>/dev/null || true
        echo "Removed service user"
    fi

    echo "ResourceAgent uninstalled successfully!"
}

# Check root
if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root"
    exit 1
fi

# Main
if $UNINSTALL; then
    uninstall_agent
else
    install_agent
fi
