# ResourceAgent Windows Installation Script
# Run as Administrator

param(
    [string]$InstallPath = "C:\Program Files\ResourceAgent",
    [string]$ConfigPath = "",
    [switch]$Uninstall
)

$ServiceName = "ResourceAgent"
$DisplayName = "ResourceAgent Monitoring Service"
$Description = "Lightweight monitoring agent for collecting hardware resource metrics"

function Install-ResourceAgent {
    Write-Host "Installing ResourceAgent..." -ForegroundColor Green

    # Create installation directory
    if (-not (Test-Path $InstallPath)) {
        New-Item -ItemType Directory -Path $InstallPath -Force | Out-Null
        Write-Host "Created installation directory: $InstallPath"
    }

    # Copy binary
    $BinarySource = Join-Path $PSScriptRoot "..\resourceagent.exe"
    if (-not (Test-Path $BinarySource)) {
        $BinarySource = ".\resourceagent.exe"
    }

    if (Test-Path $BinarySource) {
        Copy-Item $BinarySource -Destination $InstallPath -Force
        Write-Host "Copied binary to $InstallPath"
    } else {
        Write-Error "Binary not found. Please build the project first."
        exit 1
    }

    # Create configs directory
    $ConfigDir = Join-Path $InstallPath "configs"
    if (-not (Test-Path $ConfigDir)) {
        New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    }

    # Copy or create config
    if ($ConfigPath -and (Test-Path $ConfigPath)) {
        Copy-Item $ConfigPath -Destination "$ConfigDir\config.json" -Force
        Write-Host "Copied configuration from $ConfigPath"
    } else {
        $DefaultConfig = Join-Path $PSScriptRoot "..\configs\config.json"
        if (Test-Path $DefaultConfig) {
            Copy-Item $DefaultConfig -Destination "$ConfigDir\config.json" -Force
            Write-Host "Copied default configuration"
        }
    }

    # Create logs directory
    $LogDir = Join-Path $InstallPath "logs"
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
    }

    # Install service
    $BinaryPath = Join-Path $InstallPath "resourceagent.exe"
    $ConfigFile = Join-Path $ConfigDir "config.json"
    $ServicePath = "`"$BinaryPath`" -config `"$ConfigFile`""

    # Check if service exists
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "Stopping existing service..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
    }

    # Create service
    Write-Host "Creating Windows service..."
    sc.exe create $ServiceName binPath= $ServicePath start= auto DisplayName= $DisplayName | Out-Null
    sc.exe description $ServiceName $Description | Out-Null
    sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/10000/restart/30000 | Out-Null

    # Start service
    Write-Host "Starting service..."
    Start-Service -Name $ServiceName

    $service = Get-Service -Name $ServiceName
    if ($service.Status -eq "Running") {
        Write-Host "ResourceAgent installed and running successfully!" -ForegroundColor Green
    } else {
        Write-Warning "Service installed but not running. Check logs for details."
    }
}

function Uninstall-ResourceAgent {
    Write-Host "Uninstalling ResourceAgent..." -ForegroundColor Yellow

    # Stop and remove service
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Host "Stopping service..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
        Write-Host "Service removed"
    }

    # Remove installation directory (optional)
    $response = Read-Host "Remove installation directory $InstallPath? (y/N)"
    if ($response -eq "y" -or $response -eq "Y") {
        if (Test-Path $InstallPath) {
            Remove-Item -Path $InstallPath -Recurse -Force
            Write-Host "Installation directory removed"
        }
    }

    Write-Host "ResourceAgent uninstalled successfully!" -ForegroundColor Green
}

# Main
if ($Uninstall) {
    Uninstall-ResourceAgent
} else {
    Install-ResourceAgent
}
