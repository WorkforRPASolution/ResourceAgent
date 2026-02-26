#!/bin/bash
# ResourceAgent Linux Installation Script (Integrated Structure)
# Installs ResourceAgent into the shared ARSAgent basePath directory.
# Run as root or with sudo
#
# Integrated directory layout:
#   <BasePath>/bin/x86/resourceagent
#   <BasePath>/conf/ResourceAgent/{ResourceAgent,Monitor,Logging}.json
#   <BasePath>/log/ResourceAgent/
#   <BasePath>/utils/lhm-helper/  (Windows only, skipped on Linux)

set -e

BASE_PATH="/opt/EEGAgent"
SERVICE_USER="resourceagent"
UNINSTALL=false
SITE_NUM=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --base-path)
            BASE_PATH="$2"
            shift 2
            ;;
        --user)
            SERVICE_USER="$2"
            shift 2
            ;;
        --site)
            SITE_NUM="$2"
            shift 2
            ;;
        --uninstall)
            UNINSTALL=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--base-path PATH] [--user USER] [--site N] [--uninstall]"
            exit 1
            ;;
    esac
done

BIN_DIR="$BASE_PATH/bin/x86"
CONF_DIR="$BASE_PATH/conf/ResourceAgent"
LOG_DIR="$BASE_PATH/log/ResourceAgent"

install_agent() {
    echo "Installing ResourceAgent (Integrated)..."

    # Create service user if not exists
    if ! id "$SERVICE_USER" &>/dev/null; then
        useradd --system --no-create-home --shell /bin/false "$SERVICE_USER"
        echo "  Created service user: $SERVICE_USER"
    fi

    # Create directory structure
    mkdir -p "$BIN_DIR" "$CONF_DIR" "$LOG_DIR"
    echo "  Created directories under $BASE_PATH"

    # Copy binary
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    BINARY_SOURCE="$SCRIPT_DIR/../resourceagent"

    if [[ ! -f "$BINARY_SOURCE" ]]; then
        BINARY_SOURCE="./resourceagent"
    fi

    if [[ -f "$BINARY_SOURCE" ]]; then
        cp "$BINARY_SOURCE" "$BIN_DIR/resourceagent"
        chmod +x "$BIN_DIR/resourceagent"
        echo "  Copied binary to $BIN_DIR"
    else
        echo "Error: Binary not found. Please build the project first."
        exit 1
    fi

    # Copy config files
    DEFAULT_CONF="$SCRIPT_DIR/../conf/ResourceAgent"
    if [[ -d "$DEFAULT_CONF" ]]; then
        cp "$DEFAULT_CONF"/*.json "$CONF_DIR/"
        echo "  Copied default configuration files"
    fi

    # --- Site selection: configure VirtualAddressList ---
    SITES_FILE="$SCRIPT_DIR/sites.conf"
    if [[ -f "$SITES_FILE" ]]; then
        # Parse sites.conf (KEY=VALUE, skip # comments and blank lines)
        SITE_COUNT=0
        while IFS='=' read -r key val; do
            key=$(echo "$key" | tr -d '[:space:]')
            [[ -z "$key" || "$key" == \#* ]] && continue
            val=$(echo "$val" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
            eval "$key=\"$val\""
        done < "$SITES_FILE"

        if [[ "$SITE_COUNT" -gt 0 ]]; then
            SELECTED_SITE="$SITE_NUM"
            if [[ -z "$SELECTED_SITE" ]]; then
                # Interactive mode: show menu
                echo ""
                echo "=== Site Selection ==="
                for ((i=1; i<=SITE_COUNT; i++)); do
                    name_var="SITE_${i}_NAME"
                    addr_var="SITE_${i}_ADDR"
                    echo "  $i) ${!name_var} (${!addr_var})"
                done
                echo "  0) Skip (do not modify VirtualAddressList)"
                echo ""
                read -p "Select site [0-$SITE_COUNT]: " SELECTED_SITE
            fi
            if [[ "$SELECTED_SITE" == "0" ]]; then
                echo "  Site selection skipped"
            elif [[ "$SELECTED_SITE" -ge 1 && "$SELECTED_SITE" -le "$SITE_COUNT" ]]; then
                addr_var="SITE_${SELECTED_SITE}_ADDR"
                name_var="SITE_${SELECTED_SITE}_NAME"
                SITE_ADDR="${!addr_var}"
                SITE_NAME="${!name_var}"
                RA_CONFIG="$CONF_DIR/ResourceAgent.json"
                if [[ -f "$RA_CONFIG" ]]; then
                    sed -i "s|\"VirtualAddressList\": \"[^\"]*\"|\"VirtualAddressList\": \"$SITE_ADDR\"|" "$RA_CONFIG"
                    echo "  VirtualAddressList set to: $SITE_ADDR ($SITE_NAME)"
                fi
            else
                echo "ERROR: Invalid site number: $SELECTED_SITE (valid: 0-$SITE_COUNT)"
                exit 1
            fi
        fi
    fi

    # Set permissions
    chown -R "$SERVICE_USER:$SERVICE_USER" "$CONF_DIR" "$LOG_DIR"
    chown "$SERVICE_USER:$SERVICE_USER" "$BIN_DIR/resourceagent"

    # Install systemd service with absolute config paths
    cat > /etc/systemd/system/resourceagent.service << EOF
[Unit]
Description=ResourceAgent Monitoring Service
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$BASE_PATH
ExecStart=$BIN_DIR/resourceagent -config $CONF_DIR/ResourceAgent.json -monitor $CONF_DIR/Monitor.json -logging $CONF_DIR/Logging.json
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$LOG_DIR
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target
EOF

    echo "  Installed systemd service"

    # Reload systemd and start service
    systemctl daemon-reload
    systemctl enable resourceagent
    systemctl start resourceagent

    # Check status
    if systemctl is-active --quiet resourceagent; then
        echo "ResourceAgent installed and running successfully!"
        echo "  BasePath: $BASE_PATH"
        echo "  Binary:   $BIN_DIR/resourceagent"
        echo "  Config:   $CONF_DIR/"
        echo "  Logs:     $LOG_DIR/"
    else
        echo "Warning: Service installed but not running. Check: journalctl -u resourceagent"
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
    echo "  Removed systemd service"

    # Remove ResourceAgent files only (preserve ARSAgent files)
    read -p "Remove ResourceAgent files from $BASE_PATH? (y/N) " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        rm -f "$BIN_DIR/resourceagent"
        rm -rf "$CONF_DIR"
        rm -rf "$LOG_DIR"
        echo "  ResourceAgent files removed (ARSAgent files preserved)"
    fi

    # Remove service user
    read -p "Remove service user $SERVICE_USER? (y/N) " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        userdel "$SERVICE_USER" 2>/dev/null || true
        echo "  Removed service user"
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
