# coverage.ps1 — Windows 동등 (coverage.sh)
# 사용법: .\scripts\coverage.ps1 [출력디렉토리]

param([string]$OutDir = "docs/plans")

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot)

if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Force -Path $OutDir | Out-Null }

$Profile = [System.IO.Path]::GetTempFileName()
try {
    Write-Host "Running tests with coverage..."
    go test -coverprofile=$Profile -covermode=atomic ./... | Out-Null

    go tool cover -func=$Profile | Out-File -Encoding ascii (Join-Path $OutDir "coverage-baseline.txt")

    $overallLine = (go tool cover -func=$Profile)[-1]
    $overall = ($overallLine -split '\s+')[-1]
    $date = Get-Date -Format "yyyy-MM-dd"
    $gover = (go version) -split ' ' | Select-Object -Index 2

    $md = @"
# Coverage Baseline

- 측정일: $date
- Go: $gover
- 명령: ``go test -coverprofile=c.out -covermode=atomic ./...``
- **Overall: $overall**

## Phase별 목표
- Phase 2 완료 후: baseline + 2%p
- Phase 3 완료 후: baseline + 5%p
- 절대 하한: 70%

## 비고

Windows VM 측정 결과는 `coverage-baseline-windows.txt` 로 별도 저장 권장.

전체 함수별 상세는 `coverage-baseline.txt` 참조.
"@
    $md | Out-File -Encoding utf8 (Join-Path $OutDir "coverage-baseline.md")

    Write-Host "Overall coverage: $overall"
    Write-Host "Saved to:"
    Write-Host "  $OutDir/coverage-baseline.txt"
    Write-Host "  $OutDir/coverage-baseline.md"
} finally {
    Remove-Item $Profile -ErrorAction SilentlyContinue
}
