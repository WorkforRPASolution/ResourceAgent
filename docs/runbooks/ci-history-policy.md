# CI 이력 관리 정책 (CSV 기반)

GitHub Actions / 사내 CI 부재 환경에서 로컬 검증 결과를 CSV 파일로 일관 추적.

## 원칙

- 모든 머지(PR / 직접 push 무관) 전에 `./scripts/ci.sh --full` 실행 → CSV에 PASS 행 1개 들어가야 함
- CSV 파일(`docs/runbooks/ci-history.csv`) 자체를 git에 커밋
- ci.sh가 자동 append. 개발자는 `git add docs/runbooks/ci-history.csv` + 본인 커밋에 포함

## CSV 스키마

| 컬럼 | 의미 |
|------|------|
| `timestamp` | ISO 8601 UTC, 실행 시작 시각 |
| `commit` | `git rev-parse --short HEAD` (실행 시점 commit) |
| `branch` | `git branch --show-current` |
| `author` | `git config user.name` |
| `mode` | `basic` / `full` |
| `result` | `PASS` / `FAIL` |
| `failed_step` | FAIL 시 `gofmt` / `vet` / `test` / `goleak` / `cross-build:OS/ARCH`. PASS면 빈값 |
| `duration_sec` | 실행 소요 초 |
| `notes` | `--note "..."` 옵션으로 추가한 자유 텍스트 (선택) |

## 사용법

```bash
# 기본 검증 + 이력 기록
./scripts/ci.sh

# full 모드 (cross-build 포함)
./scripts/ci.sh --full

# 메모 첨부
./scripts/ci.sh --full --note "Step 1 완료"

# Windows VM
.\scripts\ci.ps1 -Full -Note "Windows 검증"
```

## CSV 분석 명령

```bash
# 마지막 10건
tail -10 docs/runbooks/ci-history.csv

# FAIL만
grep ",FAIL," docs/runbooks/ci-history.csv

# 특정 commit의 검증 여부
grep "abc1234" docs/runbooks/ci-history.csv

# 전체 PASS 율
awk -F, 'NR>1 {if($6=="PASS") p++; t++} END {if(t>0) printf "%d/%d (%.1f%%)\n", p, t, 100*p/t}' docs/runbooks/ci-history.csv

# 누가 가장 자주 실행했는지
awk -F, 'NR>1 {a[$4]++} END {for (k in a) print a[k], k}' docs/runbooks/ci-history.csv | sort -rn

# 평균 실행 시간 (모드별)
awk -F, 'NR>1 {d[$5]+=$8; c[$5]++} END {for (k in d) printf "%s: %.1fs avg (%d runs)\n", k, d[k]/c[k], c[k]}' docs/runbooks/ci-history.csv
```

## Conflict 처리

- CSV 머지 conflict 발생 시: **시간순으로 양쪽 행 모두 살리면 됨** (단순 양쪽 병합)
- 동일 timestamp는 거의 발생 안 함 (초 단위)

## 사후 감사

- 머지된 commit 중 CSV에 PASS 기록 없는 commit 발견 시 추적 + 재실행 요구
- 정기 (월 1회) PASS율 / FAIL 추이 리포트 작성

## 머지 전 권장 체크리스트

- [ ] `./scripts/ci.sh --full` 실행
- [ ] CSV에 PASS 행 추가 확인 (`tail -1 docs/runbooks/ci-history.csv`)
- [ ] CSV 변경을 본인 커밋에 함께 stage/commit
- [ ] Windows-only 코드 변경 시: Windows VM에서 `scripts/ci.ps1 -Full` 추가 실행 (같은 CSV에 append)

## CI 인프라 도입 시

향후 사내 CI(Jenkins/GitLab CI) 도입되면 `ci.sh`를 그대로 entrypoint로 사용. 동일 CSV 형식 유지하여 로컬/CI 이력 통합 추적.
