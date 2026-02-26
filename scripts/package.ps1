# ResourceAgent Windows Install Package Builder
# Creates a self-contained install package for deployment to factory PCs.
#
# Usage:
#   .\scripts\package.ps1                        # without LhmHelper
#   .\scripts\package.ps1 -IncludeLhmHelper      # with LhmHelper + PawnIO
#
# Prerequisites:
#   - ResourceAgent.exe must be built first (GOOS=windows go build ...)
#   - (optional) LhmHelper.exe must be built first (dotnet publish ...)
#
# Output:
#   install_package_windows\                     # package folder
#   install_package_windows.zip                  # compressed package

param(
    [switch]$IncludeLhmHelper
)

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path $ScriptDir -Parent
$PackageDir = Join-Path $ProjectDir "install_package_windows"
$ZipFile = Join-Path $ProjectDir "install_package_windows.zip"

Write-Host "Building ResourceAgent install package..." -ForegroundColor Green

# Clean previous package
if (Test-Path $PackageDir) {
    Remove-Item $PackageDir -Recurse -Force
}
if (Test-Path $ZipFile) {
    Remove-Item $ZipFile -Force
}

# Create package directory structure (mirrors deployment layout)
New-Item -ItemType Directory -Path (Join-Path $PackageDir "bin\x86") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $PackageDir "conf\ResourceAgent") -Force | Out-Null

# --- Copy ResourceAgent.exe ---
$Binary = Join-Path $ProjectDir "ResourceAgent.exe"
if (-not (Test-Path $Binary)) {
    Write-Error "ResourceAgent.exe not found. Build it first: GOOS=windows GOARCH=amd64 go build -o ResourceAgent.exe ./cmd/resourceagent"
    exit 1
}
Copy-Item $Binary -Destination (Join-Path $PackageDir "bin\x86\ResourceAgent.exe")
Write-Host "  Copied ResourceAgent.exe"

# --- Copy config files ---
$ConfDir = Join-Path $ProjectDir "conf\ResourceAgent"
if (-not (Test-Path $ConfDir)) {
    Write-Error "conf\ResourceAgent\ directory not found."
    exit 1
}
Copy-Item "$ConfDir\*.json" -Destination (Join-Path $PackageDir "conf\ResourceAgent\")
Write-Host "  Copied config files"

# --- Copy install scripts + guide ---
Copy-Item (Join-Path $ScriptDir "install.bat") -Destination $PackageDir
Copy-Item (Join-Path $ScriptDir "install.ps1") -Destination $PackageDir
Copy-Item (Join-Path $ScriptDir "INSTALL_GUIDE.txt") -Destination $PackageDir
Write-Host "  Copied install scripts + guide"

# --- Copy LhmHelper + PawnIO (optional) ---
if ($IncludeLhmHelper) {
    $ToolsDir = Join-Path $PackageDir "utils\lhm-helper"
    New-Item -ItemType Directory -Path $ToolsDir -Force | Out-Null

    # LhmHelper.exe
    $LhmExe = Join-Path $ProjectDir "tools\lhm-helper\bin\Release\net8.0\win-x64\publish\LhmHelper.exe"
    if (-not (Test-Path $LhmExe)) {
        Write-Error "LhmHelper.exe not found. Build it first: cd tools\lhm-helper && dotnet publish -c Release -r win-x64 --self-contained"
        exit 1
    }
    Copy-Item $LhmExe -Destination "$ToolsDir\LhmHelper.exe"
    Write-Host "  Copied LhmHelper.exe"

    # PawnIO_setup.exe
    $PawnIO = Join-Path $ProjectDir "tools\lhm-helper\PawnIO_setup.exe"
    if (-not (Test-Path $PawnIO)) {
        Write-Error "PawnIO_setup.exe not found in tools\lhm-helper\."
        exit 1
    }
    Copy-Item $PawnIO -Destination "$ToolsDir\PawnIO_setup.exe"
    Write-Host "  Copied PawnIO_setup.exe"
}

# --- Create zip ---
Compress-Archive -Path "$PackageDir\*" -DestinationPath $ZipFile -Force

Write-Host ""
Write-Host "Package created successfully!" -ForegroundColor Green
Write-Host "  Folder: $PackageDir"
Write-Host "  Zip:    $ZipFile"
Write-Host ""
Write-Host "Contents:"
Get-ChildItem $PackageDir -Recurse -File | ForEach-Object {
    Write-Host ("  " + $_.FullName.Substring($PackageDir.Length + 1))
}
