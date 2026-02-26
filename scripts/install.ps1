# ResourceAgent Windows Installation Script
# Copies files from this package to the target BasePath and registers the service.
# Run as Administrator
#
# Package layout (this script must be at the root of the package):
#   install.ps1
#   bin\x86\resourceagent.exe
#   conf\ResourceAgent\{ResourceAgent,Monitor,Logging}.json
#   tools\lhm-helper\LhmHelper.exe        (optional)
#   tools\lhm-helper\PawnIO_setup.exe      (optional)

param(
    [string]$BasePath = "D:\EARS\EEGAgent",
    [switch]$IncludeLhmHelper,
    [switch]$Uninstall
)

$ServiceName = "ResourceAgent"
$DisplayName = "ResourceAgent Monitoring Service"
$Description = "Lightweight monitoring agent for collecting hardware resource metrics"

# Package directory = where this script lives
$PkgDir = $PSScriptRoot

function Install-ResourceAgent {
    Write-Host "Installing ResourceAgent..." -ForegroundColor Green
    Write-Host "  Package: $PkgDir"
    Write-Host "  Target:  $BasePath"
    Write-Host ""

    $BinDir = Join-Path $BasePath "bin\x86"
    $ConfDir = Join-Path $BasePath "conf\ResourceAgent"
    $LogDir = Join-Path $BasePath "log\ResourceAgent"
    $ToolsDir = Join-Path $BasePath "tools\lhm-helper"

    # Create target directory structure
    foreach ($dir in @($BinDir, $ConfDir, $LogDir)) {
        if (-not (Test-Path $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
            Write-Host "  Created: $dir"
        }
    }

    # --- Copy resourceagent.exe ---
    $BinarySource = Join-Path $PkgDir "bin\x86\resourceagent.exe"
    if (-not (Test-Path $BinarySource)) {
        Write-Error "bin\x86\resourceagent.exe not found in package."
        exit 1
    }
    Copy-Item $BinarySource -Destination "$BinDir\resourceagent.exe" -Force
    Write-Host "  Copied resourceagent.exe"

    # --- Copy config files (skip if already exist at target) ---
    foreach ($file in @("ResourceAgent.json", "Monitor.json", "Logging.json")) {
        $src = Join-Path $PkgDir "conf\ResourceAgent\$file"
        $dst = Join-Path $ConfDir $file
        if (-not (Test-Path $src)) {
            Write-Error "conf\ResourceAgent\$file not found in package."
            exit 1
        }
        if (-not (Test-Path $dst)) {
            Copy-Item $src -Destination $dst -Force
            Write-Host "  Copied $file"
        } else {
            Write-Host "  Skipped $file (already exists at target)"
        }
    }

    # --- Copy LhmHelper + PawnIO (optional) ---
    if ($IncludeLhmHelper) {
        if (-not (Test-Path $ToolsDir)) {
            New-Item -ItemType Directory -Path $ToolsDir -Force | Out-Null
        }

        # Copy LhmHelper.exe
        $LhmSource = Join-Path $PkgDir "tools\lhm-helper\LhmHelper.exe"
        if (-not (Test-Path $LhmSource)) {
            Write-Error "tools\lhm-helper\LhmHelper.exe not found in package. Rebuild package with: package.sh --lhmhelper"
            exit 1
        }
        Copy-Item $LhmSource -Destination "$ToolsDir\LhmHelper.exe" -Force
        Write-Host "  Copied LhmHelper.exe"

        # Copy PawnIO_setup.exe
        $PawnioSource = Join-Path $PkgDir "tools\lhm-helper\PawnIO_setup.exe"
        if (-not (Test-Path $PawnioSource)) {
            Write-Error "tools\lhm-helper\PawnIO_setup.exe not found in package."
            exit 1
        }
        Copy-Item $PawnioSource -Destination "$ToolsDir\PawnIO_setup.exe" -Force
        Write-Host "  Copied PawnIO_setup.exe"

        # Install PawnIO driver if not already installed
        Write-Host "  Checking PawnIO driver..."
        sc.exe query PawnIO 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "  PawnIO driver not installed. Installing..."
            $process = Start-Process -FilePath "$ToolsDir\PawnIO_setup.exe" -ArgumentList "/S" -Wait -PassThru
            if ($process.ExitCode -ne 0) {
                Write-Error "PawnIO driver installation failed (exit code: $($process.ExitCode))."
                exit 1
            }
            Write-Host "  PawnIO driver installed successfully"
        } else {
            Write-Host "  PawnIO driver already installed, skipping"
        }
    }

    # --- Register Windows service ---
    $BinaryPath = Join-Path $BinDir "resourceagent.exe"
    $ConfigFile = Join-Path $ConfDir "ResourceAgent.json"
    $MonitorFile = Join-Path $ConfDir "Monitor.json"
    $LoggingFile = Join-Path $ConfDir "Logging.json"
    $ServicePath = "`"$BinaryPath`" -config `"$ConfigFile`" -monitor `"$MonitorFile`" -logging `"$LoggingFile`""

    # Remove existing service if present
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "  Stopping existing service..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
    }

    Write-Host "  Creating Windows service..."
    sc.exe create $ServiceName binPath= $ServicePath start= auto DisplayName= $DisplayName | Out-Null
    sc.exe description $ServiceName $Description | Out-Null
    sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/10000/restart/30000 | Out-Null

    Write-Host "  Starting service..."
    Start-Service -Name $ServiceName

    $service = Get-Service -Name $ServiceName
    if ($service.Status -eq "Running") {
        Write-Host ""
        Write-Host "ResourceAgent installed and running successfully!" -ForegroundColor Green
        Write-Host "  BasePath: $BasePath"
        Write-Host "  Binary:   $BinDir\resourceagent.exe"
        Write-Host "  Config:   $ConfDir\"
        Write-Host "  Logs:     $LogDir\"
    } else {
        Write-Warning "Service installed but not running. Check logs for details."
    }
}

function Uninstall-ResourceAgent {
    Write-Host "Uninstalling ResourceAgent..." -ForegroundColor Yellow

    # Stop and remove service
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "  Stopping service..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
        Write-Host "  Service removed"
    }

    # Uninstall PawnIO driver if installed
    $ToolsDir = Join-Path $BasePath "tools\lhm-helper"
    sc.exe query PawnIO 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        $PawnioSetup = Join-Path $ToolsDir "PawnIO_setup.exe"
        if (Test-Path $PawnioSetup) {
            Write-Host "  Uninstalling PawnIO driver..."
            $process = Start-Process -FilePath $PawnioSetup -ArgumentList "/S /uninstall" -Wait -PassThru
            Write-Host "  PawnIO driver uninstalled"
        } else {
            Write-Warning "PawnIO driver is installed but PawnIO_setup.exe not found. Uninstall PawnIO manually from Control Panel."
        }
    }

    # Remove ResourceAgent files only (do not touch ARSAgent files)
    $response = Read-Host "Remove ResourceAgent files from $BasePath? (y/N)"
    if ($response -eq "y" -or $response -eq "Y") {
        $BinFile = Join-Path $BasePath "bin\x86\resourceagent.exe"
        $ConfDir = Join-Path $BasePath "conf\ResourceAgent"
        $LogDir = Join-Path $BasePath "log\ResourceAgent"

        if (Test-Path $BinFile) { Remove-Item $BinFile -Force }
        if (Test-Path $ConfDir) { Remove-Item $ConfDir -Recurse -Force }
        if (Test-Path $LogDir) { Remove-Item $LogDir -Recurse -Force }
        if (Test-Path $ToolsDir) { Remove-Item $ToolsDir -Recurse -Force }
        Write-Host "  ResourceAgent files removed (ARSAgent files preserved)"
    }

    Write-Host "ResourceAgent uninstalled successfully!" -ForegroundColor Green
}

# Main
if ($Uninstall) {
    Uninstall-ResourceAgent
} else {
    Install-ResourceAgent
}
