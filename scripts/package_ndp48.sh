#!/bin/bash
# .NET Framework 4.8 Installation Package Builder
# Creates a standalone NDP48 package for sites that are authorized to install
# .NET Framework 4.8 on factory PCs.
#
# This package is intentionally separate from the main ResourceAgent installer
# so that system-level changes are never triggered implicitly during normal
# ResourceAgent deployment.
#
# Usage:
#   ./scripts/package_ndp48.sh
#
# Prerequisites:
#   - scripts/vendor/NDP48-x86-x64-AllOS-ENU.exe (~112MB, download manually)
#     See scripts/vendor/README.md for download instructions.
#
# Output:
#   install_package_ndp48/NDP48-x86-x64-AllOS-ENU.exe
#   install_package_ndp48/install_ndp48.bat
#   install_package_ndp48/README.txt
#   install_package_ndp48.zip

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PACKAGE_DIR="$PROJECT_DIR/install_package_ndp48"
NDP48_SRC="$PROJECT_DIR/scripts/vendor/NDP48-x86-x64-AllOS-ENU.exe"

if [ ! -f "$NDP48_SRC" ]; then
    echo "ERROR: $NDP48_SRC not found."
    echo "       Download from https://dotnet.microsoft.com/download/dotnet-framework/net48"
    echo "       See scripts/vendor/README.md for details."
    exit 1
fi

echo "Building .NET Framework 4.8 install package..."

if [ -d "$PACKAGE_DIR" ]; then
    rm -rf "$PACKAGE_DIR"
fi
mkdir -p "$PACKAGE_DIR"

# Copy installer
cp "$NDP48_SRC" "$PACKAGE_DIR/NDP48-x86-x64-AllOS-ENU.exe"
echo "  Copied NDP48-x86-x64-AllOS-ENU.exe"

# Create install helper batch
cat > "$PACKAGE_DIR/install_ndp48.bat" <<'EOF'
@echo off
REM .NET Framework 4.8 Installation Helper
REM Run as Administrator.
REM
REM This installer MUST only be run on PCs where system-level .NET Framework
REM install is authorized. Factory equipment PCs may have restrictions.
REM Verify with system administrator before execution.

setlocal enabledelayedexpansion

REM Check admin privileges
net session >nul 2>&1
if errorlevel 1 (
    echo ERROR: Administrator privileges required.
    echo Right-click and select "Run as administrator".
    exit /b 1
)

REM Check if already installed
set "NDP_RELEASE=0"
for /f "tokens=3" %%A in ('reg query "HKLM\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" /v Release 2^>nul ^| findstr /i "Release"') do (
    set "NDP_RELEASE=%%A"
)

if !NDP_RELEASE! GEQ 528040 (
    echo .NET Framework 4.8+ already installed ^(Release: !NDP_RELEASE!^).
    echo No action needed.
    exit /b 0
)

echo Current .NET Framework Release: !NDP_RELEASE!
echo Installing .NET Framework 4.8...
echo.

"%~dp0NDP48-x86-x64-AllOS-ENU.exe" /passive /norestart
set "RC=!errorlevel!"

if "!RC!"=="0" (
    echo.
    echo .NET Framework 4.8 installed successfully.
) else if "!RC!"=="3010" (
    echo.
    echo .NET Framework 4.8 installed. REBOOT REQUIRED.
    echo Please reboot this PC before starting any .NET 4.8 applications.
) else if "!RC!"=="1641" (
    echo.
    echo .NET Framework 4.8 installed. System will reboot shortly.
) else (
    echo.
    echo ERROR: .NET Framework 4.8 installation failed ^(exit code !RC!^).
    exit /b 1
)

echo.
echo After reboot, restart ResourceAgent service:
echo   sc.exe stop ResourceAgent
echo   sc.exe start ResourceAgent
exit /b 0
EOF
echo "  Created install_ndp48.bat"

# Create README
cat > "$PACKAGE_DIR/README.txt" <<'EOF'
========================================================
  .NET Framework 4.8 Installation Package
========================================================

This package installs .NET Framework 4.8 on the target PC.
It is distributed SEPARATELY from the main ResourceAgent installer
because factory equipment PCs may restrict system-level changes.

[When to use this package]

Install this ONLY if:
  1. The PC runs Windows 7/8/8.1 and needs LhmHelper hardware
     monitoring (temperature, fan, GPU, voltage, S.M.A.R.T).
  2. The installed .NET Framework Release is below 528040.
     Check with:
       reg query "HKLM\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" /v Release
  3. System administrator has authorized the installation.

[How to install]

  1. Copy this folder to the target PC.
  2. Right-click install_ndp48.bat -> Run as administrator.
  3. Wait for installation (~2-5 minutes).
  4. Reboot the PC if prompted.
  5. Restart ResourceAgent service:
       sc.exe stop ResourceAgent
       sc.exe start ResourceAgent

[Idempotency]

Running install_ndp48.bat on a PC that already has .NET 4.8+ is
safe - the installer detects existing versions and exits without
modifying the system.

[Version reference]

Release -> .NET Framework version:
  460798 / 460805 = 4.7
  461308 / 461310 = 4.7.1
  461808 / 461814 = 4.7.2
  528040           = 4.8        <- minimum required
  533320           = 4.8.1

========================================================
EOF
echo "  Created README.txt"

# Create zip
cd "$PROJECT_DIR"
if [ -f "install_package_ndp48.zip" ]; then
    rm "install_package_ndp48.zip"
fi
zip -r "install_package_ndp48.zip" "install_package_ndp48/"

echo ""
echo "NDP48 package created successfully!"
echo "  Folder: $PACKAGE_DIR"
echo "  Zip:    $PROJECT_DIR/install_package_ndp48.zip"
echo ""
echo "Contents:"
find "$PACKAGE_DIR" -type f | sort | sed 's|^|  |'
