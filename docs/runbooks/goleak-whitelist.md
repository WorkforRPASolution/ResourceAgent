# goleak 화이트리스트 (외부 라이브러리 long-lived goroutine)

`go.uber.org/goleak` v1.3.0 도입에 따른 `IgnoreTopFunction` 항목 근거.

## 원칙

- **외부 라이브러리의 의도된 long-lived goroutine만** 화이트리스트
- **자사 코드(`resourceagent/...`)는 절대 추가 금지** — 이후 leak 검출이 목적
- 항목 추가 PR은 리뷰에서 외부 라이브러리 검증 필수

## 패키지별 항목

### `internal/sender/sender_test.go`

| 항목 | 라이브러리 | 사유 |
|------|----------|------|
| `github.com/IBM/sarama.(*asyncProducer).dispatcher` | sarama | Kafka producer 백그라운드 dispatcher. `Close()` 호출 후에도 일부 timing race로 잔존 가능 |
| `github.com/IBM/sarama.(*asyncProducer).retryHandler` | sarama | producer 재시도 핸들러. dispatcher와 동일 |
| `github.com/IBM/sarama.(*asyncProducer).newPartitionProducer.func1` | sarama | partition별 producer 워커 |
| `github.com/IBM/sarama.(*Broker).responseReceiver` | sarama | broker 응답 수신 long-lived |
| `github.com/IBM/sarama.withRecover` | sarama | sarama 내부 패닉 복구 wrapper goroutine |
| `net/http.(*persistConn).readLoop` | net/http | HTTP keep-alive connection. `CloseIdleConnections()` 후에도 timing race 잔존 |
| `net/http.(*persistConn).writeLoop` | net/http | 동일 |
| `internal/poll.runtime_pollWait` | runtime | Go runtime network poller. 정상 |
| `gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun` | lumberjack | 로그 로테이션 cleanup goroutine. logger 인스턴스마다 생성. `Close()` API 없음 — 라이브러리 의도된 동작 |

### `internal/collector/collector_test.go`

| 항목 | 라이브러리 | 사유 |
|------|----------|------|
| `internal/poll.runtime_pollWait` | runtime | Go runtime network poller |

(추가 항목은 Step 1~4 진행 중 발견 시 PR로 추가)

## 추가 시 체크리스트

PR로 항목 추가 시 리뷰어가 확인할 것:
- [ ] Top function이 외부 라이브러리(`github.com/...`, `gopkg.in/...`, `net/...`, `runtime`, `internal/...` 등)에 속하는가?
- [ ] 자사 코드(`resourceagent/...`)가 포함되지 않는가?
- [ ] 사유가 "라이브러리의 의도된 long-lived goroutine"임이 명확한가?
- [ ] 가능하면 라이브러리 issue tracker / 소스 코드 링크 첨부

## 디버깅 절차 (false positive 발견 시)

1. `go test -v -count=1 ./internal/<패키지>/...` 실행
2. goleak 출력에서 누수 보고된 goroutine의 stack trace 확인
3. Top function 식별 (`created by ...` 줄)
4. 외부 라이브러리면 → 본 문서에 추가
5. 자사 코드면 → **수정**. 절대 ignore 하지 않음.
