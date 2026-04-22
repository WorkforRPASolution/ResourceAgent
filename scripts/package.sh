#!/bin/bash
# ResourceAgent Windows Install Package Builder
# Creates a self-contained install package for deployment to factory PCs.
#
# Usage:
#   ./scripts/package.sh                        # without LhmHelper (64-bit)
#   ./scripts/package.sh --lhmhelper             # with LhmHelper + PawnIO (64-bit)
#   ./scripts/package.sh --build                # auto-build with Go 1.20 (Win7+, 64-bit)
#   ./scripts/package.sh --build --lhmhelper     # build + LhmHelper (64-bit)
#   ./scripts/package.sh --build --arch 386      # 32-bit build (Win7 32-bit)
#
# Architecture:
#   --arch amd64   64-bit (default, Windows 7+ 64-bit)
#   --arch 386     32-bit (Windows 7+ 32-bit, LhmHelper auto-excluded)
#
# Prerequisites:
#   - ResourceAgent.exe must be built first, OR use --build flag
#   - --build requires Go 1.21+ (auto-downloads Go 1.20 toolchain via GOTOOLCHAIN)
#   - (optional) LhmHelper must be built: cd utils/lhm-helper && dotnet publish -c Release
#   - LhmHelper runs as AnyCPU but is currently packaged for 64-bit only (--arch 386 excludes it)
#
# .NET Framework 4.8 installer:
#   NOT bundled in this package. Distributed separately via ./scripts/package_ndp48.sh
#   so that factory equipment PCs can be deployed without triggering system-level installs.
#   Administrators should run NDP48 manually only when authorized.
#
# Output:
#   install_package_windows/                     # package folder (amd64)
#   install_package_windows.zip                  # compressed package (amd64)
#   install_package_windows_x86/                 # package folder (386)
#   install_package_windows_x86.zip              # compressed package (386)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
INCLUDE_LHM=false
AUTO_BUILD=false
GO_TOOLCHAIN="go1.20.14"
TARGET_ARCH="amd64"

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
        --arch)
            TARGET_ARCH="$2"
            if [[ "$TARGET_ARCH" != "amd64" && "$TARGET_ARCH" != "386" ]]; then
                echo "ERROR: --arch must be 'amd64' or '386' (got '$TARGET_ARCH')"
                exit 1
            fi
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--build] [--lhmhelper] [--arch amd64|386]"
            exit 1
            ;;
    esac
done

# 32-bit: LhmHelper is win-x64 only, auto-exclude with warning
if [[ "$TARGET_ARCH" == "386" && "$INCLUDE_LHM" == "true" ]]; then
    echo "WARNING: LhmHelper is 64-bit only. Automatically excluded for 32-bit package."
    INCLUDE_LHM=false
fi

# Set package directory and binary name based on architecture
if [[ "$TARGET_ARCH" == "386" ]]; then
    PACKAGE_DIR="$PROJECT_DIR/install_package_windows_x86"
    BINARY_NAME="ResourceAgent_x86.exe"
else
    PACKAGE_DIR="$PROJECT_DIR/install_package_windows"
    BINARY_NAME="ResourceAgent.exe"
fi

echo "Building ResourceAgent install package (arch=$TARGET_ARCH)..."

# --- Auto-build ResourceAgent.exe (optional) ---
if [ "$AUTO_BUILD" = true ]; then
    echo "  Building $BINARY_NAME with $GO_TOOLCHAIN (Windows 7+, $TARGET_ARCH)..."
    if ! command -v go &> /dev/null; then
        echo "ERROR: go command not found. Install Go 1.21+ first."
        exit 1
    fi
    # Resolve version from git tag
    BUILD_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
    BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    LDFLAGS="-X main.version=${BUILD_VERSION} -X main.buildTime=${BUILD_TIME}"
    echo "  Version: $BUILD_VERSION  BuildTime: $BUILD_TIME"
    GOTOOLCHAIN="$GO_TOOLCHAIN" GOOS=windows GOARCH="$TARGET_ARCH" \
        go build -ldflags "$LDFLAGS" -o "$PROJECT_DIR/$BINARY_NAME" ./cmd/resourceagent
    echo "  Built $BINARY_NAME successfully"
fi

# Clean previous package
if [ -d "$PACKAGE_DIR" ]; then
    rm -rf "$PACKAGE_DIR"
fi

# Create package directory structure (mirrors deployment layout)
mkdir -p "$PACKAGE_DIR/bin/x86"
mkdir -p "$PACKAGE_DIR/conf/ResourceAgent"

# --- Copy ResourceAgent.exe ---
BINARY="$PROJECT_DIR/$BINARY_NAME"
if [ ! -f "$BINARY" ]; then
    echo "ERROR: $BINARY_NAME not found."
    echo "       Build it first: GOOS=windows GOARCH=$TARGET_ARCH go build -o $BINARY_NAME ./cmd/resourceagent"
    echo "       Or use --build flag to auto-build."
    exit 1
fi
# Install as ResourceAgent.exe regardless of source name (install scripts expect this name)
cp "$BINARY" "$PACKAGE_DIR/bin/x86/ResourceAgent.exe"
echo "  Copied $BINARY_NAME → bin/x86/ResourceAgent.exe"

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
# .NET Framework 4.8 installer is NOT bundled here. It is distributed as a
# separate package (./scripts/package_ndp48.sh) so that factory equipment PCs
# do not trigger system-level installs during ResourceAgent deployment.
if [ "$INCLUDE_LHM" = true ]; then
    mkdir -p "$PACKAGE_DIR/utils/lhm-helper"

    # .NET Framework 4.7 build with Costura.Fody: all dependencies embedded into LhmHelper.exe.
    # AppendTargetFrameworkToOutputPath=false → output may be at either path.
    LHM_PUBLISH_DIR=""
    if [ -d "$PROJECT_DIR/utils/lhm-helper/bin/Release/publish" ]; then
        LHM_PUBLISH_DIR="$PROJECT_DIR/utils/lhm-helper/bin/Release/publish"
    elif [ -d "$PROJECT_DIR/utils/lhm-helper/bin/Release/net47/publish" ]; then
        LHM_PUBLISH_DIR="$PROJECT_DIR/utils/lhm-helper/bin/Release/net47/publish"
    fi

    if [ -z "$LHM_PUBLISH_DIR" ] || [ ! -f "$LHM_PUBLISH_DIR/LhmHelper.exe" ]; then
        echo "ERROR: LhmHelper publish output not found."
        echo "       Build it first: cd utils/lhm-helper && dotnet publish -c Release"
        exit 1
    fi
    cp "$LHM_PUBLISH_DIR/LhmHelper.exe" "$PACKAGE_DIR/utils/lhm-helper/"
    if [ -f "$LHM_PUBLISH_DIR/LhmHelper.exe.config" ]; then
        cp "$LHM_PUBLISH_DIR/LhmHelper.exe.config" "$PACKAGE_DIR/utils/lhm-helper/"
    fi
    echo "  Copied LhmHelper.exe (single-file with embedded dependencies)"

    PAWNIO="$PROJECT_DIR/utils/lhm-helper/PawnIO_setup.exe"
    if [ ! -f "$PAWNIO" ]; then
        echo "ERROR: PawnIO_setup.exe not found in utils/lhm-helper/."
        exit 1
    fi
    cp "$PAWNIO" "$PACKAGE_DIR/utils/lhm-helper/"
    echo "  Copied PawnIO_setup.exe"
fi

# --- Create zip ---
PACKAGE_BASENAME=$(basename "$PACKAGE_DIR")
ZIP_FILE="$PROJECT_DIR/${PACKAGE_BASENAME}.zip"
if [ -f "$ZIP_FILE" ]; then
    rm "$ZIP_FILE"
fi
(cd "$PROJECT_DIR" && zip -r "${PACKAGE_BASENAME}.zip" "${PACKAGE_BASENAME}/")
echo ""
echo "Package created successfully!"
echo "  Folder: $PACKAGE_DIR"
echo "  Zip:    $ZIP_FILE"
echo ""
echo "Contents:"
(cd "$PACKAGE_DIR" && find . -type f | sort | sed 's|^./|  |')
