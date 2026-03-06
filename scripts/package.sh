#!/bin/bash
# ResourceAgent Windows Install Package Builder
# Creates a self-contained install package for deployment to factory PCs.
#
# Usage:
#   ./scripts/package.sh                        # without LhmHelper
#   ./scripts/package.sh --lhmhelper             # with LhmHelper + PawnIO
#   ./scripts/package.sh --build                # auto-build with Go 1.20 (Win7+)
#   ./scripts/package.sh --build --lhmhelper     # build + LhmHelper
#
# Prerequisites:
#   - ResourceAgent.exe must be built first, OR use --build flag
#   - --build requires Go 1.21+ (auto-downloads Go 1.20 toolchain via GOTOOLCHAIN)
#   - (optional) LhmHelper.exe must be built first (dotnet publish ...)
#
# Output:
#   install_package_windows/                     # package folder
#   install_package_windows.zip                  # compressed package

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PACKAGE_DIR="$PROJECT_DIR/install_package_windows"
INCLUDE_LHM=false
AUTO_BUILD=false
GO_TOOLCHAIN="go1.20.14"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --lhmhelper)
            INCLUDE_LHM=true
            shift
            ;;
        --build)
            AUTO_BUILD=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--build] [--lhmhelper]"
            exit 1
            ;;
    esac
done

echo "Building ResourceAgent install package..."

# --- Auto-build ResourceAgent.exe (optional) ---
if [ "$AUTO_BUILD" = true ]; then
    echo "  Building ResourceAgent.exe with $GO_TOOLCHAIN (Windows 7+ compatible)..."
    if ! command -v go &> /dev/null; then
        echo "ERROR: go command not found. Install Go 1.21+ first."
        exit 1
    fi
    # Resolve version from git tag
    BUILD_VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    LDFLAGS="-X main.version=${BUILD_VERSION} -X main.buildTime=${BUILD_TIME}"
    echo "  Version: $BUILD_VERSION  BuildTime: $BUILD_TIME"
    GOTOOLCHAIN="$GO_TOOLCHAIN" GOOS=windows GOARCH=amd64 \
        go build -ldflags "$LDFLAGS" -o "$PROJECT_DIR/ResourceAgent.exe" ./cmd/resourceagent
    echo "  Built ResourceAgent.exe successfully"
fi

# Clean previous package
if [ -d "$PACKAGE_DIR" ]; then
    rm -rf "$PACKAGE_DIR"
fi

# Create package directory structure (mirrors deployment layout)
mkdir -p "$PACKAGE_DIR/bin/x86"
mkdir -p "$PACKAGE_DIR/conf/ResourceAgent"

# --- Copy ResourceAgent.exe ---
BINARY="$PROJECT_DIR/ResourceAgent.exe"
if [ ! -f "$BINARY" ]; then
    echo "ERROR: ResourceAgent.exe not found."
    echo "       Build it first: GOOS=windows GOARCH=amd64 go build -o ResourceAgent.exe ./cmd/resourceagent"
    exit 1
fi
cp "$BINARY" "$PACKAGE_DIR/bin/x86/"
echo "  Copied ResourceAgent.exe"

# --- Copy config files ---
CONF_DIR="$PROJECT_DIR/conf/ResourceAgent"
if [ ! -d "$CONF_DIR" ]; then
    echo "ERROR: conf/ResourceAgent/ directory not found."
    exit 1
fi
cp "$CONF_DIR"/*.json "$PACKAGE_DIR/conf/ResourceAgent/"
echo "  Copied config files"

# --- Copy install scripts + guide ---
cp "$SCRIPT_DIR/install_ResourceAgent.bat" "$PACKAGE_DIR/"
cp "$SCRIPT_DIR/install_ResourceAgent.ps1" "$PACKAGE_DIR/"
cp "$SCRIPT_DIR/INSTALL_GUIDE.txt" "$PACKAGE_DIR/"
if [ -f "$SCRIPT_DIR/sites.conf" ]; then
    cp "$SCRIPT_DIR/sites.conf" "$PACKAGE_DIR/"
    echo "  Copied sites.conf"
fi
echo "  Copied install scripts + guide"

# --- Copy LhmHelper + PawnIO (optional) ---
if [ "$INCLUDE_LHM" = true ]; then
    mkdir -p "$PACKAGE_DIR/utils/lhm-helper"

    LHM_EXE="$PROJECT_DIR/utils/lhm-helper/bin/Release/net8.0/win-x64/publish/LhmHelper.exe"
    if [ ! -f "$LHM_EXE" ]; then
        echo "ERROR: LhmHelper.exe not found."
        echo "       Build it first: cd utils/lhm-helper && dotnet publish -c Release -r win-x64 --self-contained"
        exit 1
    fi
    cp "$LHM_EXE" "$PACKAGE_DIR/utils/lhm-helper/"
    echo "  Copied LhmHelper.exe"

    PAWNIO="$PROJECT_DIR/utils/lhm-helper/PawnIO_setup.exe"
    if [ ! -f "$PAWNIO" ]; then
        echo "ERROR: PawnIO_setup.exe not found in utils/lhm-helper/."
        exit 1
    fi
    cp "$PAWNIO" "$PACKAGE_DIR/utils/lhm-helper/"
    echo "  Copied PawnIO_setup.exe"
fi

# --- Create zip ---
ZIP_FILE="$PROJECT_DIR/install_package_windows.zip"
if [ -f "$ZIP_FILE" ]; then
    rm "$ZIP_FILE"
fi
(cd "$PROJECT_DIR" && zip -r "install_package_windows.zip" "install_package_windows/")
echo ""
echo "Package created successfully!"
echo "  Folder: $PACKAGE_DIR"
echo "  Zip:    $ZIP_FILE"
echo ""
echo "Contents:"
(cd "$PACKAGE_DIR" && find . -type f | sort | sed 's|^./|  |')
