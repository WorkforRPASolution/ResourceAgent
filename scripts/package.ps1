# ResourceAgent Windows Install Package Builder
# Creates a self-contained install package for deployment to factory PCs.
#
# Usage:
#   .\scripts\package.ps1                        # without LhmHelper (64-bit)
#   .\scripts\package.ps1 -IncludeLhmHelper      # with LhmHelper + PawnIO (64-bit)
#   .\scripts\package.ps1 -Build                 # auto-build with Go 1.20 (Win7+, 64-bit)
#   .\scripts\package.ps1 -Build -IncludeLhmHelper  # build + LhmHelper (64-bit)
#   .\scripts\package.ps1 -Build -Arch 386        # 32-bit build (Win7 32-bit)
#
# Architecture:
#   -Arch amd64   64-bit (default, Windows 7+ 64-bit)
#   -Arch 386     32-bit (Windows 7+ 32-bit, LhmHelper auto-excluded)
#
# Prerequisites:
#   - ResourceAgent.exe must be built first, OR use -Build flag
#   - -Build requires Go 1.21+ (auto-downloads Go 1.20 toolchain via GOTOOLCHAIN)
#   - (optional) LhmHelper.exe must be built first (dotnet publish ...)
#   - LhmHelper is 64-bit only; automatically excluded for -Arch 386
#
# Output:
#   install_package_windows\                     # package folder (amd64)
#   install_package_windows.zip                  # compressed package (amd64)
#   install_package_windows_x86\                 # package folder (386)
#   install_package_windows_x86.zip              # compressed package (386)

param(
    [switch]$IncludeLhmHelper,
    [switch]$Build,
    [ValidateSet("amd64", "386")]
    [string]$Arch = "amd64"
)

$GoToolchain = "go1.20.14"

$ErrorActionPreference = "Stop"

# 32-bit: LhmHelper is win-x64 only, auto-exclude with warning
if ($Arch -eq "386" -and $IncludeLhmHelper) {
    Write-Host "WARNING: LhmHelper is 64-bit only. Automatically excluded for 32-bit package." -ForegroundColor Yellow
    $IncludeLhmHelper = $false
}

# Set package directory and binary name based on architecture
if ($Arch -eq "386") {
    $BinaryName = "ResourceAgent_x86.exe"
    $PackageSuffix = "_x86"
} else {
    $BinaryName = "ResourceAgent.exe"
    $PackageSuffix = ""
}

$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path $ScriptDir -Parent
$PackageDir = Join-Path $ProjectDir "install_package_windows$PackageSuffix"
$ZipFile = Join-Path $ProjectDir "install_package_windows$PackageSuffix.zip"

Write-Host "Building ResourceAgent install package (arch=$Arch)..." -ForegroundColor Green

# --- Auto-build ResourceAgent.exe (optional) ---
if ($Build) {
    Write-Host "  Building $BinaryName with $GoToolchain (Windows 7+, $Arch)..."
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        Write-Error "go command not found. Install Go 1.21+ first."
        exit 1
    }
    # Resolve version from git tag
    try { $BuildVersion = & git describe --tags --always --dirty 2>$null } catch { $BuildVersion = $null }
    if (-not $BuildVersion) { $BuildVersion = "dev" }
    $BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $Ldflags = "-X main.version=$BuildVersion -X main.buildTime=$BuildTime"
    Write-Host "  Version: $BuildVersion  BuildTime: $BuildTime"
    $env:GOTOOLCHAIN = $GoToolchain
    $env:GOOS = "windows"
    $env:GOARCH = $Arch
    & go build -ldflags "$Ldflags" -o (Join-Path $ProjectDir $BinaryName) ./cmd/resourceagent
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build $BinaryName"
        exit 1
    }
    # Clean up environment variables
    Remove-Item Env:\GOTOOLCHAIN -ErrorAction SilentlyContinue
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Write-Host "  Built $BinaryName successfully"
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
$Binary = Join-Path $ProjectDir $BinaryName
if (-not (Test-Path $Binary)) {
    Write-Error "$BinaryName not found. Build it first: GOOS=windows GOARCH=$Arch go build -o $BinaryName ./cmd/resourceagent`nOr use -Build flag to auto-build."
    exit 1
}
# Install as ResourceAgent.exe regardless of source name (install scripts expect this name)
Copy-Item $Binary -Destination (Join-Path $PackageDir "bin\x86\ResourceAgent.exe")
Write-Host "  Copied $BinaryName -> bin\x86\ResourceAgent.exe"

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
