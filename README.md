# Debate TUI (Go)

Bubble Tea 기반 TUI 앱으로 OpenAI Responses API를 사용해 멀티 페르소나 토론을 실행합니다.

## Requirements

- Go 1.23+
- `OPENAI_API_KEY` (필수)

## Run

```bash
go run ./cmd/debate
```

앱 시작 시 기본으로 `./personas.json`을 로드합니다.

- stdin/stdout이 TTY면 TUI 모드 실행
- TTY가 아니면 자동으로 REPL 모드로 fallback

## Environment Variables

기본값은 코드의 `internal/config/config.go`와 동일합니다.

- `OPENAI_API_KEY` (required)
- `OPENAI_BASE_URL` (optional)
- `OPENAI_MODEL` (default: `gpt-5.2`)
- `DEBATE_MAX_TURNS` (default: `0`, `0`이면 무제한)
- `DEBATE_CONSENSUS_THRESHOLD` (default: `0.85`, range: `0..1`)
- `DEBATE_MAX_DURATION` (default: `20m`, duration format)
- `DEBATE_MAX_TOTAL_TOKENS` (default: `120000`, `> 0`)
- `DEBATE_MAX_NO_PROGRESS_JUDGE` (default: `6`, `> 0`)
- `OPENAI_REQUEST_TIMEOUT` (default: `60s`, duration format)
- `OPENAI_API_MAX_RETRIES` (default: `2`, `>= 0`)

## Debate Behavior

- 페르소나가 순환하며 발언합니다.
- 페르소나 발언 사이마다 `사회자`가 중간 정리 및 다음 발화 유도를 합니다.
- 토론 종료 시 마지막은 항상 `사회자`의 최종 총정리/총평으로 끝납니다.

### Termination Conditions

아래 중 하나에 도달하면 종료됩니다.

- 합의 도달: `consensus_reached`
- 최대 턴 도달: `max_turns_reached`
- 최대 시간 도달: `duration_limit_reached`
- 최대 토큰 도달: `token_limit_reached`
- 합의 점수 정체 도달: `no_progress_reached`
- 오류: `error`

## TUI Usage

- 입력창에 명령어 입력 후 `Enter`
- `Ctrl+C` 종료
- `Ctrl+P / Ctrl+N` 명령 히스토리
- `Ctrl+F` auto-follow 토글
- `PgUp / PgDn / Home / End` 로그 스크롤
- `Mouse wheel / trackpad` 로그 스크롤
- `Ctrl+L` 로그 패널 초기화

### TUI Commands

- `/ask <problem>` 토론 실행
- `/stop` 실행 중 토론 중지
- `/follow [on|off|toggle]` auto-follow 제어
- `/show` 로드된 페르소나 출력
- `/load` `personas.json` 다시 로드
- `/help` 도움말
- `/exit` 종료

편의 입력:

- `ask <problem>` (슬래시 없이) 가능
- 슬래시 없는 일반 문장 입력 시 `/ask`로 처리

## REPL Usage

REPL에서는 아래 명령만 지원합니다.

- `/ask <problem>`
- `/show`
- `/load`
- `/help`
- `/exit`

편의 입력:

- `ask <problem>` (슬래시 없이) 가능
- 슬래시 없는 일반 문장 입력 시 `/ask`로 처리

## Output

- 각 `/ask` 세션 결과는 `./outputs/*.json`으로 저장됩니다.
- 결과 JSON에는 토론 턴, 합의 정보, 상태, 토큰 사용량, 시작/종료 시간이 포함됩니다.

## Persona File Schema

`personas.json`은 페르소나 객체 배열이어야 합니다.

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

Validation rules:

- 2 to 12 personas
- `id`, `name`, `role` 필수
- `master_name` optional (롤모델/참고 인물 이름)
- `id`는 unique
- `signature_lens`는 optional (권장)

`master_name`를 설정하면 해당 인물의 공개적으로 알려진 지식/저술/논문/아티클 기반 관점을 우선 반영하도록 프롬프트가 강화됩니다(실존 인물 사칭 금지).
