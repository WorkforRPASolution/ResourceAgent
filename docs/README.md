# ResourceAgent 문서 인덱스

모든 문서의 지도입니다. 상태 태그로 유효성을 한눈에 파악할 수 있습니다.

> 🟢 **현행** — 살아있는 문서, 항상 최신 유지
> 🔵 **연구·대기** — 리서치 완료, 실행 대기 또는 참고용
> ⚫ **아카이브** — 결정 완료·시점성 기록, 수정하지 않음

## 문서 배치 규칙

| 폴더 | 용도 | 파일명 규칙 |
|------|------|------------|
| `reference/` | 살아있는 참조 문서 (기능·명세·분석) | `UPPERCASE.md` |
| `runbooks/` | 운영·현장 대응 절차 + 개발 기록 | `kebab-case.md` — **코드가 경로를 참조하므로 이동 금지** |
| `issues/` | 알려진 제약·이슈 포스트모템 | `kebab-case.md` |
| `field/` | 현장 관리자 배포용 HTML (비개발자 대상) | `kebab-case.html` |
| `plans/` | 진행 중 계획 (완료 시 `archive/`로 이동) | `kebab-case.md` |
| `research/` | 리서치·비교 분석 | `kebab-case.md` |
| `archive/` | 결정 완료·시점성 문서 | `YYYY-MM-DD-kebab.md` (작성일 prefix) |

## reference/ — 참조 문서 🟢

| 문서 | 설명 |
|------|------|
| [COLLECTORS.md](reference/COLLECTORS.md) | 15종 수집기 상세 설명·설정·출력 예시 (최중요 레퍼런스) |
| [EARS-METRICS-REFERENCE.md](reference/EARS-METRICS-REFERENCE.md) | EARS Grok 포맷 메트릭 명세 (category/metric/proc/value) |
| [RESOURCE-USAGE.md](reference/RESOURCE-USAGE.md) | 에이전트 자체 CPU/메모리/디스크/네트워크 사용량 분석 |
| [DATA-VOLUME-ESTIMATION.md](reference/DATA-VOLUME-ESTIMATION.md) | Kafka→Elasticsearch 적재 데이터량 산정 |
| [AGENT-HEALTH.md](reference/AGENT-HEALTH.md) | Redis 기반 heartbeat (Watchdog) 메커니즘 |
| [TESTING-LHM-TEMPERATURE.md](reference/TESTING-LHM-TEMPERATURE.md) | Windows CPU 온도 수집 테스트 가이드 |

## runbooks/ — 운영 절차 🟢

| 문서 | 설명 |
|------|------|
| [boot-recovery-hotfix.md](runbooks/boot-recovery-hotfix.md) | 재부팅 후 서비스 미시작 핫픽스 사용 가이드 |
| [windows-memory-leak-diagnosis.md](runbooks/windows-memory-leak-diagnosis.md) | Windows 메모리 누수 진단 절차 (Pool/Driver Locked 분리 식별) |
| [lhm-provider-timeout-monitoring.md](runbooks/lhm-provider-timeout-monitoring.md) | LhmProvider timeout 모니터링 (Phase 1-1) |
| [wmi-query-monitoring.md](runbooks/wmi-query-monitoring.md) | WMI StorageHealth 쿼리 모니터링 (Phase 1-2) |
| [buffered-http-transport-monitoring.md](runbooks/buffered-http-transport-monitoring.md) | KafkaRest 버퍼 모니터링 (Phase 2-1) |
| [selfmetrics-overview.md](runbooks/selfmetrics-overview.md) | SelfMetrics 7종 지표 운영 가이드 (Phase 2.5-1) |

**개발 기록** (runbook은 아니지만 테스트 코드가 경로를 참조하여 이곳에 유지):

| 문서 | 설명 |
|------|------|
| [goleak-whitelist.md](runbooks/goleak-whitelist.md) | goleak IgnoreTopFunction 화이트리스트 근거 |
| [known-race-conditions.md](runbooks/known-race-conditions.md) | `go test -race` 발견 race 목록 |
| [known-windows-test-failures.md](runbooks/known-windows-test-failures.md) | Windows 환경 테스트 skip 목록 (테스트 skip 메시지가 참조) |

## issues/ — 알려진 제약·이슈 🟢

| 문서 | 설명 |
|------|------|
| [pawnio-windows7-incompatible.md](issues/pawnio-windows7-incompatible.md) | PawnIO 2.0.1.0 Win7 비호환 → WinRing0 fallback 근거 |
| [win7-net8-modified-memory.md](issues/win7-net8-modified-memory.md) | LhmHelper .NET 8→4.7 전환 근거 (Win7 Modified 메모리 폭증) |
| [kafkarest-wsasend-connection-aborted.md](issues/kafkarest-wsasend-connection-aborted.md) | KafkaRest wsasend 간헐 에러 분석 |
| [file-sender-drain-console-race.md](issues/file-sender-drain-console-race.md) | FileSender drainConsole race (RESOLVED) |

## field/ — 현장 배포용 안내 (비개발자 대상 HTML) 🟢

| 문서 | 설명 |
|------|------|
| [startup-cpu-spike-explained.html](field/startup-cpu-spike-explained.html) | 시작 시 CPU 순간 상승(~30%)이 정상인 이유 |
| [cpu-vs-taskmanager-explained.html](field/cpu-vs-taskmanager-explained.html) | CPU 사용률이 작업 관리자와 다른 이유 (Processor Time vs Utility) |

## plans/ — 진행 중 계획 🟢

| 문서 | 설명 |
|------|------|
| [memory-leak-mitigation-plan.md](plans/memory-leak-mitigation-plan.md) | 메모리 누수 대응 마스터 플랜 (v2.4.2, Phase별 진행 기록) |

## research/ — 리서치 🔵

| 문서 | 설명 |
|------|------|
| [macos-porting-research.md](research/macos-porting-research.md) | macOS 포팅 검토 (리서치 완료, 실행 대기) |
| [ohmgraphite-comparison.md](research/ohmgraphite-comparison.md) | OhmGraphite 비교 분석 (2026-02, LHM 확장 방향 결정에 활용됨) |

## archive/ — 완료된 기록 ⚫

| 문서 | 설명 |
|------|------|
| [2026-02-05-lhm-hardware-extension.report.md](archive/2026-02-05-lhm-hardware-extension.report.md) | LHM 하드웨어 확장 완료 보고서 (GPU/SMART/전압/MB온도) |
| [2026-02-26-collection-interval-review.md](archive/2026-02-26-collection-interval-review.md) | 수집 주기 검토 — **결정 완료** (B안 + TopN=10 채택) |
| [2026-02-27-lhm-daemon-bugfix.report.md](archive/2026-02-27-lhm-daemon-bugfix.report.md) | LhmHelper daemon 모드 동시성 버그 수정 보고서 |
| [2026-03-02-kafka-sender-review.md](archive/2026-03-02-kafka-sender-review.md) | Kafka sender 유지/삭제 검토 — **결정 완료** (유지) |
| [2026-04-28-eps-whitelist-request.md](archive/2026-04-28-eps-whitelist-request.md) | EPS 화이트리스트 협의 자료 — **해결 완료** (등록으로 메모리 증가 해소) |
| [2026-05-04-resource-management-audit.md](archive/2026-05-04-resource-management-audit.md) | 메모리/소켓/핸들 관리 audit — leak 0건 확인 |

## 루트 문서

| 문서 | 설명 |
|------|------|
| [../README.md](../README.md) | 프로젝트 개요·빌드·설정·설치 (진입점) |
| [../ResourceAgent_PRD.md](../ResourceAgent_PRD.md) | 제품 요구사항 문서 |
| [../CLAUDE.md](../CLAUDE.md) | Claude Code 작업 가이드 |
| [../scripts/INSTALL_GUIDE.txt](../scripts/INSTALL_GUIDE.txt) | 현장 담당자용 설치 가이드 (설치 패키지에 동봉) |
