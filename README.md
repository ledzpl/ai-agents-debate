# Debate TUI (Go)

Bubble Tea 기반 TUI/REPL 앱으로 OpenAI Responses API를 사용해 멀티 페르소나 토론을 실행합니다.

## 핵심 기능

- 멀티 페르소나 순환 토론 + 사회자 개입 + 합의 판정
- TUI(터미널 인터랙티브) / REPL(비대화형 환경 fallback) 자동 전환
- `master_name` 기반 롤모델 지식 반영 프롬프트
- 토론 결과 JSON 자동 저장 (`./outputs`)
- persona 수가 많아도 패널이 compact/overflow-safe로 렌더링

## 요구 사항

- Go 1.23+
- `OPENAI_API_KEY` (필수)

## 실행

```bash
export OPENAI_API_KEY="<your-key>"
go run ./cmd/debate
```

기본 로딩 경로:

- persona 파일: `./personas.json`
- 결과 저장: `./outputs`

## 실행 모드

- stdin/stdout이 TTY이면 TUI 모드
- TTY가 아니면 REPL 모드

## 환경 변수

기본값은 `internal/config/config.go` 기준입니다.

| 변수 | 기본값 | 설명 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 없음 | OpenAI API 키 (필수) |
| `OPENAI_BASE_URL` | 없음 | 커스텀 엔드포인트 베이스 URL |
| `OPENAI_MODEL` | `gpt-5.2` | 사용할 모델 |
| `DEBATE_MAX_TURNS` | `0` | persona 턴 최대치 (`0` = 무제한) |
| `DEBATE_CONSENSUS_THRESHOLD` | `0.85` | 합의 점수 임계값 (`0..1`) |
| `DEBATE_MAX_DURATION` | `20m` | 최대 실행 시간 (duration 형식) |
| `DEBATE_MAX_TOTAL_TOKENS` | `120000` | 최대 누적 토큰 (`> 0`) |
| `DEBATE_MAX_NO_PROGRESS_JUDGE` | `6` | 합의 점수 정체 허용 횟수 (`> 0`) |
| `OPENAI_REQUEST_TIMEOUT` | `60s` | API 요청 타임아웃 |
| `OPENAI_API_MAX_RETRIES` | `2` | 재시도 횟수 (`>= 0`) |

## 토론 동작

1. persona가 순환하면서 발언
2. persona 발언 사이마다 사회자가 요약/질문으로 개입
3. 주기적으로 합의 점수 판정
4. 종료 시 마지막은 항상 사회자 최종 정리 턴

### 종료 상태

- `consensus_reached`
- `max_turns_reached`
- `duration_limit_reached`
- `token_limit_reached`
- `no_progress_reached`
- `error`

## TUI 사용법

단축키:

- `Enter`: 명령 실행
- `Ctrl+C`: 종료
- `Ctrl+P` / `Ctrl+N`: 명령 히스토리
- `Ctrl+F`: auto-follow 토글
- `PgUp` / `PgDn` / `Home` / `End`: 로그 스크롤
- `Mouse wheel` / `trackpad`: 로그 스크롤
- `Ctrl+L`: 로그 패널 초기화

명령어:

- `/ask <problem>` 토론 실행
- `/stop` 실행 중 토론 중지
- `/follow [on|off|toggle]` auto-follow 제어
- `/show` 로드된 persona 출력
- `/load` `personas.json` 재로드
- `/help` 도움말
- `/exit` 종료

편의 입력:

- `ask <problem>` (슬래시 없이) 허용
- 슬래시 없는 일반 문장은 자동으로 `/ask` 처리

## REPL 사용법

REPL 지원 명령:

- `/ask <problem>`
- `/show`
- `/load`
- `/help`
- `/exit`

편의 입력:

- `ask <problem>` (슬래시 없이) 허용
- 슬래시 없는 일반 문장은 자동으로 `/ask` 처리

## 결과 파일

- 각 토론 결과는 `./outputs/*-debate.json`으로 저장됩니다.
- JSON에는 problem/personas/turns/consensus/status/metrics/timestamps가 포함됩니다.

## persona 스키마

`personas.json`은 persona 객체 배열입니다.

```json
[
  {
    "id": "architect",
    "name": "System Architect",
    "master_name": "Martin Fowler",
    "role": "long-term scalability",
    "stance": "cautious",
    "style": "structured",
    "expertise": ["distributed systems"],
    "signature_lens": ["frame decisions by clear tradeoffs"],
    "constraints": ["Mention tradeoffs"]
  }
]
```

검증 규칙:

- persona 수는 2~12
- `id`, `name`, `role` 필수
- `id`는 unique
- `stance` 미입력 시 `neutral`
- `expertise` / `signature_lens` / `constraints`는 빈값 trim 후 정규화

`name` 과 `master_name`:

- `name`: 토론 역할명(페르소나 이름)
- `master_name`: 참고 롤모델 이름(선택)
- UI/프롬프트에서는 필요 시 `name (master_name)` 형태로 표시

`master_name`가 있으면 프롬프트가 해당 인물의 공개 지식/저술/논문/아티클 관점을 반영하도록 강화됩니다.  
실존 인물 사칭은 금지되며, 불확실한 서지정보를 지어내지 않도록 가드레일이 포함됩니다.

## 샘플 persona 세트

샘플 파일은 `./exmaples` 디렉터리에 있습니다.  
참고: 디렉터리명은 현재 코드 기준으로 `exmaples`입니다.

- `personas.brainstorming.json`
- `personas.company.json`
- `personas.friend.json`
- `personas.ideas.json`
- `personas.pm.json`

사용 예시:

```bash
cp ./exmaples/personas.pm.json ./personas.json
go run ./cmd/debate
```

## 테스트

```bash
go test ./...
go vet ./...
```
