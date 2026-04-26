#!/usr/bin/env bash
# coverage.sh — 패키지별 coverage 측정. baseline 비교용.
#
# 사용법:
#   ./scripts/coverage.sh [출력디렉토리]
#
# 출력디렉토리 기본: docs/plans
#   - coverage-baseline.txt  (전체 함수별 표)
#   - coverage-baseline.md   (요약 + 측정일)

set -euo pipefail
cd "$(dirname "$0")/.."

OUTDIR="${1:-docs/plans}"
mkdir -p "$OUTDIR"

TMP_PROFILE="$(mktemp -t resourceagent-cov.XXXXXX)"
trap "rm -f '$TMP_PROFILE'" EXIT

echo "Running tests with coverage..."
go test -coverprofile="$TMP_PROFILE" -covermode=atomic ./... > /dev/null

go tool cover -func="$TMP_PROFILE" > "$OUTDIR/coverage-baseline.txt"

OVERALL=$(go tool cover -func="$TMP_PROFILE" | tail -1 | awk '{print $3}')
DATE=$(date +%Y-%m-%d)
GOVER=$(go version | awk '{print $3}')

cat > "$OUTDIR/coverage-baseline.md" <<EOF
# Coverage Baseline

- 측정일: $DATE
- Go: $GOVER
- 명령: \`go test -coverprofile=c.out -covermode=atomic ./...\`
- **Overall: $OVERALL**

## Phase별 목표
- Phase 2 완료 후: baseline + 2%p
- Phase 3 완료 후: baseline + 5%p
- 절대 하한: 70%

## 패키지별 합계 (cover/statements 비율)

| 패키지 | Statements | Covered | % |
|--------|-----------|---------|---|
EOF

# 패키지별 라인 합계 (statements 가중 평균)
go tool cover -func="$TMP_PROFILE" | grep -v "^total:" | awk '
{
    pkg = $1
    sub(/\/[^\/]+:[0-9]+:$/, "", pkg)
    sub(/:[0-9]+:$/, "", pkg)
    # 함수 수 카운트만 — statements 가중치는 cover profile 직접 파싱 필요. 단순 평균.
    pct = $NF
    sub(/%/, "", pct)
    pcts[pkg] += pct
    counts[pkg]++
}
END {
    for (pkg in pcts) {
        if (counts[pkg] > 0) {
            avg = pcts[pkg] / counts[pkg]
            printf "| %s | %d functions | - | %.1f%% (function avg) |\n", pkg, counts[pkg], avg
        }
    }
}
' | sort >> "$OUTDIR/coverage-baseline.md"

cat >> "$OUTDIR/coverage-baseline.md" <<EOF

## 미커버 핫스팟 (보강 우선순위)

\`\`\`
EOF
go tool cover -func="$TMP_PROFILE" | awk '$NF=="0.0%" {print $0}' | head -30 >> "$OUTDIR/coverage-baseline.md"
echo '```' >> "$OUTDIR/coverage-baseline.md"

cat >> "$OUTDIR/coverage-baseline.md" <<EOF

## 비고

- Windows-only 코드(\`*_windows.go\`)는 macOS 측정에서 제외됨. Windows VM에서 별도 측정 시 \`coverage-baseline-windows.txt\` 로 저장 권장.
- 통합 테스트(\`-tags=integration\`) 미포함.

전체 함수별 상세는 \`coverage-baseline.txt\` 참조.
EOF

echo "Overall coverage: $OVERALL"
echo "Saved to:"
echo "  $OUTDIR/coverage-baseline.txt"
echo "  $OUTDIR/coverage-baseline.md"
