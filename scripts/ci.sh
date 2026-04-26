#!/usr/bin/env bash
# ci.sh — 로컬 CI 검증. 결과는 docs/runbooks/ci-history.csv 에 자동 기록.
#
# 사용법:
#   ./scripts/ci.sh [--full] [--note "메모"]
#
# --full: race + 다중 OS cross-build 검증 포함 (느림, ~5분)
# 기본:   gofmt + vet + test(race) + goleak smoke (~1분)

set -uo pipefail
cd "$(dirname "$0")/.."

FULL=0
NOTES=""
MODE="basic"
START=$(date +%s)
RESULT="FAIL"
FAILED_STEP=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --full) FULL=1; MODE="full"; shift ;;
        --note) NOTES="${2:-}"; shift 2 ;;
        -h|--help)
            sed -n '2,10p' "$0"
            exit 0 ;;
        *) shift ;;
    esac
done

# CSV 이력 append (PASS/FAIL 양쪽에서 trap으로 호출)
record_history() {
    local ts commit branch author duration safe_notes
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    branch=$(git branch --show-current 2>/dev/null || echo "unknown")
    author=$(git config user.name 2>/dev/null || echo "unknown")
    duration=$(( $(date +%s) - START ))
    safe_notes=$(printf '%s' "$NOTES" | tr -d '\n,' | head -c 200)

    mkdir -p docs/runbooks
    if [[ ! -f docs/runbooks/ci-history.csv ]]; then
        echo "timestamp,commit,branch,author,mode,result,failed_step,duration_sec,notes" \
            > docs/runbooks/ci-history.csv
    fi
    echo "$ts,$commit,$branch,$author,$MODE,$RESULT,$FAILED_STEP,$duration,$safe_notes" \
        >> docs/runbooks/ci-history.csv

    echo ""
    echo "Recorded to docs/runbooks/ci-history.csv ($RESULT, ${duration}s)"
    if [[ "$RESULT" == "PASS" ]]; then
        echo "Don't forget: git add docs/runbooks/ci-history.csv"
    fi
}
trap record_history EXIT

echo "=== Step 1/5: gofmt ==="
UNFORMATTED=$(gofmt -l . | grep -v '^vendor/' || true)
if [[ -n "$UNFORMATTED" ]]; then
    echo "FAIL: unformatted files:"
    echo "$UNFORMATTED"
    FAILED_STEP="gofmt"
    exit 1
fi
echo "PASS"

echo "=== Step 2/5: go vet ==="
if ! go vet ./...; then
    FAILED_STEP="vet"
    exit 1
fi
echo "PASS"

echo "=== Step 3/5: go test ==="
if ! go test -count=1 -timeout=10m ./...; then
    FAILED_STEP="test"
    exit 1
fi
echo "PASS"

echo "=== Step 4/5: goleak smoke ==="
# TestMain 안의 goleak이 모든 패키지에서 실행됨 (위 Step 3에 포함).
echo "PASS (goleak runs via TestMain)"

if [[ $FULL -eq 1 ]]; then
    echo "=== Step 5a/5: go test -race (full) ==="
    if ! go test -race -count=1 -timeout=15m ./...; then
        FAILED_STEP="race"
        exit 1
    fi
    echo "PASS"

    echo "=== Step 5b/5: cross-build (full) ==="
    for combo in "windows amd64" "windows 386" "linux amd64"; do
        read -r goos goarch <<< "$combo"
        echo "  Build $goos/$goarch"
        if ! GOOS=$goos GOARCH=$goarch go build -o /tmp/resourceagent-build-check ./cmd/resourceagent; then
            FAILED_STEP="cross-build:$goos/$goarch"
            exit 1
        fi
        rm -f /tmp/resourceagent-build-check
    done
    echo "PASS"
else
    echo "=== Step 5/5: race + cross-build SKIPPED (use --full) ==="
fi

RESULT="PASS"
echo ""
echo "=== ALL PASSED ==="
