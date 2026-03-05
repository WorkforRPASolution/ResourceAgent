# ResourceAgent Windows Install Package Builder
# Creates a self-contained install package for deployment to factory PCs.
#
# Usage:
#   .\scripts\package.ps1                        # without LhmHelper
#   .\scripts\package.ps1 -IncludeLhmHelper      # with LhmHelper + PawnIO
#   .\scripts\package.ps1 -Build                 # auto-build with Go 1.20 (Win7+)
#   .\scripts\package.ps1 -Build -IncludeLhmHelper  # build + LhmHelper
#
# Prerequisites:
#   - ResourceAgent.exe must be built first, OR use -Build flag
#   - -Build requires Go 1.21+ (auto-downloads Go 1.20 toolchain via GOTOOLCHAIN)
#   - (optional) LhmHelper.exe must be built first (dotnet publish ...)
#
# Output:
#   install_package_windows\                     # package folder
#   install_package_windows.zip                  # compressed package

param(
    [switch]$IncludeLhmHelper,
    [switch]$Build
)

$GoToolchain = "go1.20.14"

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path $ScriptDir -Parent
$PackageDir = Join-Path $ProjectDir "install_package_windows"
$ZipFile = Join-Path $ProjectDir "install_package_windows.zip"

Write-Host "Building ResourceAgent install package..." -ForegroundColor Green

# --- Auto-build ResourceAgent.exe (optional) ---
if ($Build) {
    Write-Host "  Building ResourceAgent.exe with $GoToolchain (Windows 7+ compatible)..."
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        Write-Error "go command not found. Install Go 1.21+ first."
        exit 1
    }
    $env:GOTOOLCHAIN = $GoToolchain
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    & go build -o (Join-Path $ProjectDir "ResourceAgent.exe") ./cmd/resourceagent
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build ResourceAgent.exe"
        exit 1
    }
    # Clean up environment variables
    Remove-Item Env:\GOTOOLCHAIN -ErrorAction SilentlyContinue
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Write-Host "  Built ResourceAgent.exe successfully"
}

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
Copy-Item (Join-Path $ScriptDir "install_ResourceAgent.bat") -Destination $PackageDir
Copy-Item (Join-Path $ScriptDir "install_ResourceAgent.ps1") -Destination $PackageDir
Copy-Item (Join-Path $ScriptDir "INSTALL_GUIDE.txt") -Destination $PackageDir
$SitesConf = Join-Path $ScriptDir "sites.conf"
if (Test-Path $SitesConf) {
    Copy-Item $SitesConf -Destination $PackageDir
    Write-Host "  Copied sites.conf"
}
Write-Host "  Copied install scripts + guide"

# --- Copy LhmHelper + PawnIO (optional) ---
if ($IncludeLhmHelper) {
    $ToolsDir = Join-Path $PackageDir "utils\lhm-helper"
    New-Item -ItemType Directory -Path $ToolsDir -Force | Out-Null

    # LhmHelper.exe
    $LhmExe = Join-Path $ProjectDir "utils\lhm-helper\bin\Release\net8.0\win-x64\publish\LhmHelper.exe"
    if (-not (Test-Path $LhmExe)) {
        Write-Error "LhmHelper.exe not found. Build it first: cd utils\lhm-helper && dotnet publish -c Release -r win-x64 --self-contained"
        exit 1
    }
    Copy-Item $LhmExe -Destination "$ToolsDir\LhmHelper.exe"
    Write-Host "  Copied LhmHelper.exe"

    # PawnIO_setup.exe
    $PawnIO = Join-Path $ProjectDir "utils\lhm-helper\PawnIO_setup.exe"
    if (-not (Test-Path $PawnIO)) {
        Write-Error "PawnIO_setup.exe not found in utils\lhm-helper\."
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
