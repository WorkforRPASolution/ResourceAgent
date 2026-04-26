# ci.ps1 — Windows 동등 (ci.sh)
# 사용법:
#   .\scripts\ci.ps1 [-Full] [-Note "메모"]

param(
    [switch]$Full,
    [string]$Note = ""
)

$ErrorActionPreference = "Continue"
Set-Location (Split-Path $PSScriptRoot)

$Mode = if ($Full) { "full" } else { "basic" }
$Start = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$Script:Result = "FAIL"
$Script:FailedStep = ""

function Record-History {
    $ts = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $commit = try { (git rev-parse --short HEAD) } catch { "unknown" }
    $branch = try { (git branch --show-current) } catch { "unknown" }
    $author = try { (git config user.name) } catch { "unknown" }
    $duration = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds() - $Start
    $safeNotes = ($Note -replace '[\n,]', '').Substring(0, [Math]::Min(200, $Note.Length))

    if (-not (Test-Path "docs/runbooks")) { New-Item -ItemType Directory -Force -Path "docs/runbooks" | Out-Null }
    if (-not (Test-Path "docs/runbooks/ci-history.csv")) {
        "timestamp,commit,branch,author,mode,result,failed_step,duration_sec,notes" | Out-File -Encoding ascii "docs/runbooks/ci-history.csv"
    }
    "$ts,$commit,$branch,$author,$Mode,$($Script:Result),$($Script:FailedStep),$duration,$safeNotes" | Out-File -Append -Encoding ascii "docs/runbooks/ci-history.csv"

    Write-Host ""
    Write-Host "Recorded to docs/runbooks/ci-history.csv ($($Script:Result), ${duration}s)"
}

try {
    Write-Host "=== Step 1/5: gofmt ==="
    $unformatted = gofmt -l . | Where-Object { $_ -notmatch '^vendor/' }
    if ($unformatted) {
        Write-Host "FAIL: unformatted files:"
        $unformatted | ForEach-Object { Write-Host $_ }
        $Script:FailedStep = "gofmt"
        exit 1
    }
    Write-Host "PASS"

    Write-Host "=== Step 2/5: go vet ==="
    go vet ./...
    if ($LASTEXITCODE -ne 0) { $Script:FailedStep = "vet"; exit 1 }
    Write-Host "PASS"

    Write-Host "=== Step 3/5: go test ==="
    go test -count=1 -timeout=10m ./...
    if ($LASTEXITCODE -ne 0) { $Script:FailedStep = "test"; exit 1 }
    Write-Host "PASS"

    Write-Host "=== Step 4/5: goleak smoke ==="
    Write-Host "PASS (goleak runs via TestMain)"

    if ($Full) {
        Write-Host "=== Step 5a/5: go test -race (full) ==="
        go test -race -count=1 -timeout=15m ./...
        if ($LASTEXITCODE -ne 0) { $Script:FailedStep = "race"; exit 1 }
        Write-Host "PASS"

        Write-Host "=== Step 5b/5: cross-build (full) ==="
        @(@{goos='windows';goarch='amd64'}, @{goos='windows';goarch='386'}, @{goos='linux';goarch='amd64'}) | ForEach-Object {
            $env:GOOS = $_.goos
            $env:GOARCH = $_.goarch
            Write-Host "  Build $($_.goos)/$($_.goarch)"
            $output = "$env:TEMP\resourceagent-build-check"
            go build -o $output ./cmd/resourceagent
            if ($LASTEXITCODE -ne 0) {
                $Script:FailedStep = "cross-build:$($_.goos)/$($_.goarch)"
                exit 1
            }
            Remove-Item $output -ErrorAction SilentlyContinue
        }
        Write-Host "PASS"
    } else {
        Write-Host "=== Step 5/5: race + cross-build SKIPPED (use -Full) ==="
    }

    $Script:Result = "PASS"
    Write-Host ""
    Write-Host "=== ALL PASSED ==="
} finally {
    Record-History
}
