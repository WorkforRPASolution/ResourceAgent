# ResourceAgent 재부팅 미시작 핫픽스 사용 가이드

대상 스크립트: [`scripts/fix_ResourceAgent_boot_recovery.bat`](../../scripts/fix_ResourceAgent_boot_recovery.bat)

이미 배포된 PC에서 **"재부팅 후 ResourceAgent 서비스가 시작되지 않는"** 문제를, 재설치/파일 교체 없이 **설정 변경만으로 즉시 완화**하는 현장 핫픽스입니다.

---

## 1. 언제 쓰나 (적용 대상)

다음 조건에 해당하는 PC에 적용합니다.

- `sender_type = kafkarest` (네트워크 의존 startup 경로를 탐)
- Windows 7 등에서 **재부팅 후 서비스가 멈춘 채 올라오지 않음**
- 로그(`log\ResourceAgent\ResourceAgent.log`)에 부팅 직후 다음과 유사한 흔적:
  ```
  [WRN] DetectIPByDial failed ... unreachable host
  [ERR] [windows-service] Service failed during startup
        error="failed to fetch EQP_INFO from Redis: ... unreachable host"
  ```
  그리고 그 뒤로 `Starting ResourceAgent` 가 **다시 나타나지 않음**(= 자동 재시작이 안 됨).

> `sender_type = file` PC에는 해당하지 않습니다(네트워크 startup 경로를 타지 않음).

---

## 2. 무엇이 문제이고, 이 스크립트가 무엇을 바꾸나

**원인(2층 구조)**
1. **트리거** — 부팅 직후 Redis/ServiceDiscovery 호스트로 가는 **경로가 아직 없어**(`WSAEHOSTUNREACH`, "unreachable host") startup이 1회 실패.
2. **방치(핵심 결함)** — 서비스가 `SERVICE_STOPPED`(종료코드≠0)로 **정상 종료 보고**하는데, `FailureActionsFlag` 기본값(0)에서는 SCM이 이를 "정상 종료"로 간주해 **재시작 복구를 발동하지 않음** → 다음 재부팅까지 정지.

**스크립트 적용 내용**

| 단계 | 명령 | 효과 |
|---|---|---|
| 1 | `sc failure ... 5000/10000/30000/60000/300000` | 복구 액션(재시작 5s→10s→30s→1m→5m, 이후 5m 반복) 보장 |
| **2** | **`sc failureflag <svc> 1`** | **핵심** — 정상 종료(STOPPED, 에러코드)에도 복구 발동 |
| 3 | `sc config <svc> start= delayed-auto` | 자동(지연 시작)으로 변경 → 부팅 시 네트워크 준비 후 기동 (`/noauto`로 생략) |
| 4 | (멈춰 있으면) `sc start <svc>` | 재부팅 없이 즉시 복구 (`/nostart`로 생략) |

---

## 3. 사전 조건

- **관리자 권한**으로 실행 (스크립트가 `net session`으로 확인하며, 아니면 중단).
- 대상 PC에 ResourceAgent 서비스가 **설치되어 있어야 함**(스크립트가 `sc query`로 확인).
- `sc failureflag` / `start= delayed-auto` 는 **Windows 7 이상** 지원.

---

## 4. 사용법

관리자 권한 명령 프롬프트에서 실행합니다.

```cmd
REM 기본: failureflag + 복구액션 + 지연시작 적용, 멈춰 있으면 즉시 시작
fix_ResourceAgent_boot_recovery.bat

REM 서비스명이 다를 경우 (기본값: ResourceAgent)
fix_ResourceAgent_boot_recovery.bat MyServiceName

REM 시작 유형을 자동(지연)으로 바꾸지 않고 기존 시작 유형 유지
REM (부팅 후 ~2분 지연을 원치 않을 때)
fix_ResourceAgent_boot_recovery.bat /noauto

REM 지금 당장 서비스를 시작하지는 않음 (설정만 적용)
fix_ResourceAgent_boot_recovery.bat /nostart
```

옵션은 조합 가능합니다. 예: `fix_ResourceAgent_boot_recovery.bat /noauto /nostart`

| 인자 | 의미 |
|---|---|
| `<서비스명>` | 위치 인자. 생략 시 `ResourceAgent` |
| `/noauto` | 시작 유형 변경(3단계) 생략 — 기존 시작 유형 유지 |
| `/nostart` | 현재 서비스 시작(4단계) 생략 — 설정만 적용 |

**종료 코드:** `0` = 적용 완료, `1` = 미적용(관리자 아님 / 서비스 없음).

---

## 5. 실행 결과 예시 (정상)

```
============================================================
 ResourceAgent Boot Recovery Hotfix
 Service: ResourceAgent
============================================================

[1/4] Registering failure actions (restart 5s/10s/30s/1m/5m)...
  OK
[2/4] Setting FailureActionsFlag=1 (recover even on graceful STOPPED)...
  OK
[3/4] Setting start type to Automatic (Delayed Start)...
  OK
[4/4] Checking service state...
  Stopped - starting now...
  OK: service is running.

============================================================
 Result
============================================================
        START_TYPE         : 2   AUTO_START  (DELAYED)
[SC] QueryServiceConfig2 SUCCESS

FAILURE_ACTIONS_FLAG : TRUE

Done. From the next boot, a startup failure will auto-restart (self-heals).
Verify above: FAILURE_ACTIONS_FLAG should be TRUE.
```

확인 포인트: **`FAILURE_ACTIONS_FLAG : TRUE`** 와 `START_TYPE ... (DELAYED)`.

---

## 6. 적용 확인 (수동 검증)

스크립트 출력 외에 직접 확인하려면:

```cmd
sc qc ResourceAgent
sc qfailureflag ResourceAgent
sc query ResourceAgent
```

- `sc qc` → `START_TYPE : 2 AUTO_START (DELAYED)` (또는 `/noauto` 시 `AUTO_START`)
- `sc qfailureflag` → `FAILURE_ACTIONS_FLAG : TRUE`
- `sc query` → `STATE : 4 RUNNING`

**재부팅 검증:** 대상 PC를 재부팅한 뒤 `sc query ResourceAgent` 가 (지연 시작 + 필요 시 복구 재시작을 거쳐) **RUNNING** 이 되는지 확인합니다.

---

## 7. 다수 PC 배포

- **개별 실행:** 각 PC에 `.bat`을 복사하고 관리자 권한으로 실행.
- **원격 실행(PsExec 예):**
  ```cmd
  psexec \\<PC> -s -c fix_ResourceAgent_boot_recovery.bat
  ```
  (`-s` = SYSTEM 권한으로 실행, `-c` = 로컬 파일 복사 후 실행)
- **공유 배포:** ManagerAgent/FTP 등 기존 배포 경로로 `.bat`을 내려보낸 뒤 원격 실행.

> 설정만 변경하므로 서비스 중지/재설치가 필요 없고, **여러 번 실행해도 안전**(idempotent)합니다.

---

## 8. 되돌리기 (rollback)

```cmd
sc config ResourceAgent start= auto      REM 지연 시작 -> 즉시 자동으로 복원
sc failureflag ResourceAgent 0           REM 복구 플래그 해제(기본값)
```

> 복구 액션 자체(restart 5s/10s/30s/1m/5m)는 그대로 두어도 무방합니다(크래시 시에만 추가로 동작).

---

## 9. 주의사항 / 한계

- ⚠️ **이건 즉시 완화책입니다(자가복구 보장), 근본 해결이 아닙니다.** 부팅 시 네트워크가 늦으면 `시작 → 실패 → 재시작` **플래핑**이 잠깐 발생하며 이벤트 로그/`ResourceAgent.log`에 startup 실패가 수 회 남을 수 있습니다(네트워크가 올라오면 정상화).
- **재설치 시:** 업데이트된 `install_ResourceAgent.bat`은 이제 **`failureflag=1` + 점증 재시작 액션(5s/10s/30s/1m/5m) + `start= delayed-auto`를 기본 포함**하므로, 재설치해도 자가복구·지연 시작이 모두 유지됩니다(핫픽스 재실행 불필요). (구버전 설치본으로 설치된 PC는 이들 설정이 없으므로 이 핫픽스 적용이 필요합니다.)
- **지속적 도달 불가인 경우엔 효과 없음:** 부팅 후에도 그 PC에서 Redis 호스트에 도달하지 못한다면(아래 확인) 이건 서비스가 아니라 **네트워크/방화벽/Redis 인프라** 문제입니다.
  ```cmd
  ping <redis_ip>
  powershell -Command "Test-NetConnection <redis_ip> -Port <redis_port>"
  ```
- **근본 해결(다음 릴리스):** startup의 Redis/ServiceDiscovery 초기화를 **재시도/백오프**로 감싸 부팅 시 네트워크 미준비를 견디게 하는 것입니다(런타임 heartbeat가 이미 쓰는 패턴). 그와 함께 설치 스크립트에 `failureflag=1`을 기본 포함하면 이 핫픽스가 불필요해집니다.

---

## 10. 문제 해결 (FAQ)

| 증상 | 원인 / 조치 |
|---|---|
| `WARNING: failed to set failureflag` | 해당 OS의 `sc.exe`가 미지원(매우 드묾). Win7 이상이면 지원. `sc qfailureflag <svc>`로 수동 확인 |
| 적용 후에도 `not RUNNING yet` | 실행 시점에 아직 네트워크 미준비. 복구 정책으로 자동 재시작됨. 잠시 후 `sc query`로 재확인, `ResourceAgent.log` 점검 |
| 재부팅 후에도 계속 실패 | 그 PC에서 Redis 호스트 **지속 도달 불가** 가능성 → 9절의 `ping`/`Test-NetConnection`으로 확인 후 네트워크/인프라 점검 |
| 부팅 시 로그에 startup 실패가 몇 줄 남음 | 정상(플래핑). 네트워크가 올라온 뒤 RUNNING으로 안정화되는지만 확인 |

---

## 관련 문서

- 설치 스크립트: [`scripts/install_ResourceAgent.bat`](../../scripts/install_ResourceAgent.bat)
- 핫픽스 스크립트: [`scripts/fix_ResourceAgent_boot_recovery.bat`](../../scripts/fix_ResourceAgent_boot_recovery.bat)
