# .NET Framework 4.8 Installation Package Builder (PowerShell)
# Creates a standalone NDP48 package for sites that are authorized to install
# .NET Framework 4.8 on factory PCs.
#
# This package is intentionally separate from the main ResourceAgent installer
# so that system-level changes are never triggered implicitly during normal
# ResourceAgent deployment.
#
# Usage:
#   .\scripts\package_ndp48.ps1
#
# Prerequisites:
#   - scripts\vendor\NDP48-x86-x64-AllOS-ENU.exe (~112MB, download manually)
#     See scripts\vendor\README.md for download instructions.
#
# Output:
#   install_package_ndp48\NDP48-x86-x64-AllOS-ENU.exe
#   install_package_ndp48\install_ndp48.bat
#   install_package_ndp48\README.txt
#   install_package_ndp48.zip

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path $ScriptDir -Parent
$PackageDir = Join-Path $ProjectDir "install_package_ndp48"
$ZipFile = Join-Path $ProjectDir "install_package_ndp48.zip"
$Ndp48Src = Join-Path $ProjectDir "scripts\vendor\NDP48-x86-x64-AllOS-ENU.exe"

if (-not (Test-Path $Ndp48Src)) {
    Write-Error "$Ndp48Src not found.`nDownload from https://dotnet.microsoft.com/download/dotnet-framework/net48`nSee scripts\vendor\README.md for details."
    exit 1
}

Write-Host "Building .NET Framework 4.8 install package..." -ForegroundColor Green

if (Test-Path $PackageDir) {
    Remove-Item $PackageDir -Recurse -Force
}
if (Test-Path $ZipFile) {
    Remove-Item $ZipFile -Force
}
New-Item -ItemType Directory -Path $PackageDir -Force | Out-Null

# Copy installer
Copy-Item $Ndp48Src -Destination (Join-Path $PackageDir "NDP48-x86-x64-AllOS-ENU.exe")
Write-Host "  Copied NDP48-x86-x64-AllOS-ENU.exe"

# Copy helper scripts (reuse the files created by package_ndp48.sh or generate)
$InstallBat = @'
@echo off
REM .NET Framework 4.8 Installation Helper
REM Run as Administrator.

setlocal enabledelayedexpansion

net session >nul 2>&1
if errorlevel 1 (
    echo ERROR: Administrator privileges required.
    exit /b 1
)

set "NDP_RELEASE=0"
for /f "tokens=3" %%A in ('reg query "HKLM\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" /v Release 2^>nul ^| findstr /i "Release"') do (
    set "NDP_RELEASE=%%A"
)

if !NDP_RELEASE! GEQ 528040 (
    echo .NET Framework 4.8+ already installed ^(Release: !NDP_RELEASE!^).
    exit /b 0
)

echo Current .NET Framework Release: !NDP_RELEASE!
echo Installing .NET Framework 4.8...
echo.

"%~dp0NDP48-x86-x64-AllOS-ENU.exe" /passive /norestart
set "RC=!errorlevel!"

if "!RC!"=="0" (
    echo .NET Framework 4.8 installed successfully.
) else if "!RC!"=="3010" (
    echo .NET Framework 4.8 installed. REBOOT REQUIRED.
) else if "!RC!"=="1641" (
    echo .NET Framework 4.8 installed. System will reboot shortly.
) else (
    echo ERROR: .NET Framework 4.8 installation failed ^(exit code !RC!^).
    exit /b 1
)

echo.
echo After reboot, restart ResourceAgent service:
echo   sc.exe stop ResourceAgent
echo   sc.exe start ResourceAgent
exit /b 0
'@
Set-Content -Path (Join-Path $PackageDir "install_ndp48.bat") -Value $InstallBat -Encoding ASCII
Write-Host "  Created install_ndp48.bat"

$Readme = @'
========================================================
  .NET Framework 4.8 Installation Package
========================================================

This package installs .NET Framework 4.8 on the target PC.
It is distributed SEPARATELY from the main ResourceAgent installer
because factory equipment PCs may restrict system-level changes.

[When to use this package]

Install this ONLY if:
  1. The PC needs LhmHelper hardware monitoring.
  2. The installed .NET Framework Release is below 528040.
  3. System administrator has authorized the installation.

[How to install]

  1. Copy this folder to the target PC.
  2. Right-click install_ndp48.bat -> Run as administrator.
  3. Reboot if prompted.
  4. Restart ResourceAgent service:
       sc.exe stop ResourceAgent
       sc.exe start ResourceAgent

[Idempotency]

Running install_ndp48.bat on a PC that already has .NET 4.8+ is safe.

========================================================
'@
Set-Content -Path (Join-Path $PackageDir "README.txt") -Value $Readme -Encoding ASCII
Write-Host "  Created README.txt"

# Create zip
Compress-Archive -Path "$PackageDir\*" -DestinationPath $ZipFile -Force

Write-Host ""
Write-Host "NDP48 package created successfully!" -ForegroundColor Green
Write-Host "  Folder: $PackageDir"
Write-Host "  Zip:    $ZipFile"
Write-Host ""
Write-Host "Contents:"
Get-ChildItem $PackageDir -Recurse -File | ForEach-Object {
    Write-Host ("  " + $_.FullName.Substring($PackageDir.Length + 1))
}
