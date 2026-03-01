# Debate Web (Go)

OpenAI Responses API를 사용해 멀티 페르소나 토론을 실행하는 웹 애플리케이션입니다.

## 핵심 기능

- 멀티 페르소나 순환 토론 + 사회자 개입(필요 시 생략) + 합의 판정
- 웹 UI에서 SSE 기반 턴 스트리밍
- 좌측 persona 그룹 선택 + persona 목록(`master_name` 포함) 표시
- 우측 턴 단위 토론 타임라인 + 현재 화자 하이라이트
- 토론 실행 중 progress bar 노출
- 토론 결과 JSON + Markdown 자동 저장 (`./outputs`)

## 요구 사항

- Go 1.23+
- `OPENAI_API_KEY` (필수)

## 실행

```bash
export OPENAI_API_KEY="<your-key>"
go run ./cmd/debate
```

기본 웹 서버 주소는 `http://localhost:8080` 입니다.

옵션:

- `--personas` 또는 `--persona`: persona JSON 경로 지정
- `--addr`: 서버 listen 주소 (예: `:8090`)

예시:

```bash
go run ./cmd/debate --personas ./exmaples/personas.pm.json --addr :8090
```

기본 경로:

- persona 파일: `./personas.json`
- 결과 저장: `./outputs`

## 웹 엔드포인트

- `GET /`: 웹 UI (`internal/web/static/index.html`)
- `GET /static/*`: 정적 자산 (`app.css`, `app.js`)
- `GET /api/personas?path=./personas.json`
- `POST /api/debate`
- `POST /api/debate/stream/start` (run 생성)
- `GET /api/debate/stream?run_id=...` (SSE 구독)
- `POST /api/debate/stream/stop` (run 중지)

`POST /api/debate` 요청 규칙:

- JSON body 필드: `problem`(필수), `persona_path`(선택), `personas`(선택)
- unknown field는 거부됩니다.
- 여러 JSON 값을 이어 붙인 body는 거부됩니다.

`POST /api/debate/stream/start` 요청 규칙:

- JSON body 스키마는 `POST /api/debate`와 동일합니다.
- 응답으로 `run_id`를 반환하며, 이후 `GET /api/debate/stream?run_id=...`로 구독합니다.

`POST /api/debate/stream/stop` 요청 규칙:

- JSON body 필드: `run_id`(필수)

SSE 이벤트 타입:

- `start`: 토론 시작 메타 정보
- `turn`: 생성된 각 토론 턴
- `complete`: 최종 결과 + 저장 경로
- `stopped`: 사용자 중지 요청으로 종료
- `debate_error`: 실행/저장 오류

## 보안 제약

persona 경로는 아래 제약을 만족해야 합니다.

- `.json` 파일만 허용
- 프로젝트 디렉터리 내부 경로만 허용 (path traversal 차단)

## 환경 변수

기본값은 `internal/config/config.go` 기준입니다.

| 변수 | 기본값 | 설명 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 없음 | OpenAI API 키 (필수) |
| `OPENAI_BASE_URL` | 없음 | 커스텀 엔드포인트 베이스 URL |
| `OPENAI_MODEL` | `gpt-5.2` | 사용할 모델 |
| `DEBATE_MAX_TURNS` | `0` | persona 턴 최대치 (`0` = 무제한) |
| `DEBATE_CONSENSUS_THRESHOLD` | `0.90` | 합의 점수 임계값 (`0..1`) |
| `DEBATE_MAX_DURATION` | `20m` | 최대 실행 시간 (duration 형식) |
| `DEBATE_MAX_TOTAL_TOKENS` | `120000` | 최대 누적 토큰 (`> 0`) |
| `DEBATE_MAX_NO_PROGRESS_JUDGE` | `6` | 합의 점수 정체 허용 횟수 (`> 0`) |
| `OPENAI_REQUEST_TIMEOUT` | `60s` | API 요청 타임아웃 |
| `OPENAI_API_MAX_RETRIES` | `2` | API 재시도 횟수 (`>= 0`) |

## 토론 동작

1. persona가 순환하면서 발언합니다.
2. 기본적으로는 persona 발언 사이에 사회자가 요약/질문으로 개입합니다.
3. 단, persona 발화 말미에 `NEXT: <persona_id>`가 명시되면 사회자 턴을 건너뛰고 해당 persona로 직접 핸드오프합니다.
4. 사회자 없이 진행되는 구간에서는 각 persona가 말미에 `CLOSE: yes|no`, `NEW_POINT: yes|no`를 함께 남기며, `close 합의 + 신규 논점 정체`가 감지되면 조기 종료할 수 있습니다.
5. 라운드 단위로 합의 점수를 판정하며, 사회자 없는 연속 구간에서는 판정 빈도를 높입니다.
6. 합의는 임계값 1회가 아닌 연속 판정(기본 2회)으로 확인 후 종료합니다.
7. 종료 시 마지막은 항상 사회자 최종 정리 턴입니다.

### 종료 상태

- `consensus_reached`
- `max_turns_reached`
- `duration_limit_reached`
- `token_limit_reached`
- `no_progress_reached`
- `error`

## 결과 파일

각 토론 결과는 아래 2개 파일로 저장됩니다.

- `./outputs/*-debate.json`
- `./outputs/*-debate.md`

JSON에는 `problem/personas/turns/consensus/status/metrics/timestamps`가 포함됩니다.

Markdown에는 `problem/consensus/personas/turns/metrics`가 읽기 좋은 형태로 정리됩니다.

- `## Turns`에 turn 순서 TOC 링크 포함
- 화자별 묶음은 `<details open>`으로 접기/펼치기 가능

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
- `expertise` / `signature_lens` / `constraints`는 trim 후 빈값 제거

## 샘플 persona 세트

샘플 파일은 `./exmaples` 디렉터리에 있습니다.
참고: 디렉터리명은 현재 코드 기준으로 `exmaples`입니다.

- `personas.brainstorming.json`
- `personas.company.json`
- `personas.friend.json`
- `personas.ideas.json`
- `personas.music.json`
- `personas.pm.json`
- `personas.sec.json`

## 테스트

```bash
go test ./...
go vet ./...
```
