# KafkaRest wsasend Connection Aborted 간헐적 에러

## 현상

KafkaRest 방식으로 메트릭 전송 시 간헐적으로 다음 경고 로그 발생:

```
[WRN] [buffered-kafkar] Buffered KafkaRest send failed, retrying attempt=1
error="HTTP request failed: Post \"http://\": readfrom tcp 11.99.127.158:61242->11.99.127.158:30000:
write tcp 11.99.127.158:61242->11.99.127.158:30000:
wsasend: An established connection was aborted by the software in your host machine."
records=16
```

- Windows 에러 코드: `WSAECONNABORTED` (10053)
- 재시도 후 정상 전송됨 (attempt=1)

## 원인

Go `http.Client`의 **커넥션 풀링과 서버측 idle timeout 미스매치** 문제.

1. Go HTTP Transport가 유휴 커넥션을 풀에 보관 (현재 `IdleConnTimeout: 90s`)
2. KafkaRest 프록시(서버)가 이보다 짧은 시간에 유휴 커넥션을 종료
3. 클라이언트가 이미 닫힌 커넥션에 write 시도 → `wsasend` 에러

Windows 환경에서 방화벽/보안 소프트웨어가 장시간 유휴 TCP 커넥션을 강제 종료하는 경우에도 동일 증상 발생 가능.

## 실제 영향: 없음

BufferedHTTPTransport에 재시도 로직이 구현되어 있어 **데이터 유실 없이 자동 복구**된다.
간헐적 WARN 로그만 남을 뿐 기능에는 영향 없음.

## 관련 코드

| 파일 | 설정 |
|------|------|
| `internal/network/socks.go` | `IdleConnTimeout: 90s`, `MaxIdleConnsPerHost: 10` |
| `internal/sender/kafkarest.go` | `http.Client{Timeout: 10s}`, 재시도 로직 포함 |

## 조치 방법

### 방법 1: IdleConnTimeout 단축 (권장)

`internal/network/socks.go`의 `IdleConnTimeout`을 서버보다 짧게 설정하여 클라이언트가 먼저 유휴 커넥션을 정리하도록 변경.

```go
// 변경 전
IdleConnTimeout: 90 * time.Second,

// 변경 후 (30초로 단축)
IdleConnTimeout: 30 * time.Second,
```

- 에러 발생 빈도를 줄일 수 있음
- 커넥션 재생성 비용은 무시할 수 있는 수준

### 방법 2: 현 상태 유지

- 재시도 로직이 정상 동작하므로 추가 조치 없이도 문제 없음
- WARN 로그가 간헐적으로 남는 것을 수용

### 방법 3: Keep-Alive 비활성화

```go
transport := &http.Transport{
    DisableKeepAlives: true,
}
```

- 매 요청마다 새 커넥션을 생성하여 stale connection 문제 원천 차단
- 커넥션 생성 오버헤드 증가 (수집 주기가 길면 영향 미미)

## 결론

**추가 조치 불필요.** 재시도 로직이 정상 동작하여 데이터 유실 없음.
WARN 로그 빈도를 줄이고 싶다면 방법 1(IdleConnTimeout 30s 단축)을 적용.

## 상태

- [x] 원인 분석 완료 (2026-03-09)
- [ ] 조치 여부 결정
