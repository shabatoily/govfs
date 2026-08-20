# MCP 지원 설계

## 목적

govfs의 파일 시스템 기능을 AI 에이전트가 MCP(Model Context Protocol) 도구로 사용할 수 있게 한다. 초기 구현은 기존 CLI 바이너리의 stdio transport만 지원하며, 별도 MCP 바이너리나 저장소 접근 계층은 만들지 않는다.

## 범위

초기 버전은 다음 기능만 제공한다.

| MCP 도구 | 입력 | 동작 |
| --- | --- | --- |
| `vfs_tree` | `path` | 지정한 경로 이하의 트리를 조회한다. |
| `vfs_stat` | `id` | 파일 또는 디렉터리의 메타데이터를 조회한다. |
| `vfs_mkdir` | `path` | 디렉터리를 생성한다. |
| `vfs_upload` | `path`, `content_base64` | base64로 전달된 파일을 업로드한다. |
| `vfs_delete` | `id` | 파일 또는 디렉터리를 삭제한다. |

파일 수정, 이동, 복사, 백업 및 복원은 실제 사용 요구가 생길 때 추가한다. 애플리케이션에서 제거된 cloud 기능은 MCP에서도 제공하지 않는다.

## 구조

```text
AI Agent
    │ stdio
    ▼
govfs mcp
    │
    ▼
internal/mcp
    │ 기존 internal/client 사용
    ▼
govfs HTTP API
    ▼
VFS Service → VFS Driver
```

- `internal/mcp`는 MCP 서버 생성과 도구 등록 및 실행을 담당한다.
- `internal/cli`는 `govfs mcp` 명령을 등록하고 stdio transport를 시작한다.
- `cmd/cli`는 기존 CLI 진입점을 유지한다.
- MCP 프로세스는 VFS 드라이버나 Badger DB를 직접 열지 않는다. 기존 HTTP API를 사용하여 인증과 서버 동작 경계를 유지한다.
- MCP SDK는 공식 Go SDK `github.com/modelcontextprotocol/go-sdk` v1.6.1을 사용한다.

예상 파일 구성은 다음과 같다.

```text
internal/mcp/
├── server.go
├── tools.go
└── tools_test.go

internal/cli/mcp.go
```

초기 구현에서 transport 추상화나 단일 구현용 인터페이스는 만들지 않는다. HTTP transport가 실제로 추가될 때 공통 코드가 확인되면 분리한다.

## 실행 및 설정

MCP 클라이언트는 기존 CLI 바이너리를 하위 프로세스로 실행한다.

```json
{
  "command": "govfs",
  "args": ["mcp"]
}
```

`govfs mcp`는 `govfs login`이 `~/.govfs/config`에 저장한 서버 주소와 access token을 재사용한다. stdio는 MCP 메시지 전용이므로 다음 규칙을 지킨다.

- stdout에는 MCP 프로토콜 메시지만 출력한다.
- 로그와 진단 메시지는 stderr에 출력한다.
- 설정이 없거나 인증에 실패하면 대화형 입력을 요청하지 않고 오류로 종료한다.
- 종료 신호와 MCP 클라이언트 연결 종료 시 실행 중인 요청을 취소하고 정상 종료한다.

## 도구 동작

### 조회

`vfs_tree`와 `vfs_stat`은 기존 `internal/client.VFSClient`의 조회 메서드를 사용하고 JSON 구조화 결과를 반환한다. URL, UUID, 경로, 파일 크기, 디렉터리 여부 및 수정 시간을 에이전트가 후속 호출에 사용할 수 있어야 한다.

### 업로드

`vfs_upload`는 `content_base64`를 디코딩한 스트림을 기존 multipart 업로드 API에 전달한다.

- 디코딩된 파일이 10 MiB를 초과하면 업로드 전에 거부한다.
- 입력 경로에서 로컬 파일을 직접 읽지 않는다.
- 올바르지 않은 base64와 빈 경로를 검증한다.
- 업로드 성공 시 생성된 메타데이터를 반환한다.

로컬 경로 입력은 MCP 프로세스의 임의 파일 접근 범위를 넓히므로 초기 범위에서 제외한다.

### 삭제

`vfs_delete`는 UUID만 입력받으며 파괴적 도구로 표시한다. 성공 응답은 요청 접수가 아니라 실제 삭제 완료를 의미해야 한다.

현재 VFS 변경 API는 작업을 비동기로 실행하고 `202 Accepted`를 반환한다. 유효한 `X-Client-ID`가 있으면 성공 또는 실패 결과를 해당 SSE 구독자에게 알리며, 없으면 작업만 실행한다. MCP 프로세스는 시작할 때 SSE 연결 하나를 열고 발급받은 client ID를 이후 변경 요청에 사용한다.

초기 구현에는 공통 작업 ID가 없으므로 MCP 변경 도구를 프로세스 안에서 직렬 실행하고, 30초 안에 도착한 해당 action의 완료 이벤트를 기다린다. 생성 완료 후에는 이벤트의 ID로 `stat`을 호출하여 최종 메타데이터를 반환한다. 동시 변경 요청을 지원해야 할 때 HTTP 응답과 SSE 이벤트에 공통 작업 ID를 추가한다.

## 보안

- MCP는 기존 govfs 인증을 우회하지 않는다.
- 토큰, 비밀번호 및 파일 내용은 로그에 남기지 않는다.
- 모든 경로와 UUID는 신뢰 경계에서 검증한다.
- 업로드 크기를 제한하여 과도한 메모리 사용을 막는다.
- 삭제 도구에는 MCP의 파괴적 작업 메타데이터를 설정한다.

## 오류 처리

도구 오류는 에이전트가 수정 가능한 입력 오류와 서버 오류를 구분할 수 있는 짧은 메시지로 반환한다. HTTP 상태 코드만 노출하지 않고 작업, 대상 및 원인을 포함하며 인증 정보나 파일 내용을 포함하지 않는다.

## 향후 HTTP 지원

원격 에이전트 지원이 필요해지면 기존 Fiber 서버에 MCP HTTP 엔드포인트를 추가할 수 있다. 이때 `internal/mcp`의 도구 정의와 실행 코드를 재사용하고 transport 시작 부분만 추가한다.

공식 Go MCP SDK의 Streamable HTTP 구현은 `net/http` 기반이므로 Fiber 연결 방식은 구현 시점에 다음 선택지를 비교한다.

- Fiber와 `net/http` 사이의 얇은 어댑터 사용
- 동일 프로세스에서 별도의 MCP HTTP listener 실행
- 별도 MCP HTTP 프로세스가 기존 govfs API 호출

HTTP endpoint, OAuth, 세션 저장 및 다중 사용자 격리는 원격 사용 요구가 확정되기 전에는 구현하지 않는다.

## 완료 기준

- `govfs mcp`가 stdio MCP 서버로 실행된다.
- 설정 과정에서 stdin을 사용하지 않는다.
- 다섯 개 초기 도구의 목록과 입력 스키마가 노출된다.
- 조회, 디렉터리 생성, 이미지 업로드 및 삭제가 실제 govfs 서버를 대상으로 검증된다.
- 업로드 크기 제한과 삭제 완료 여부가 테스트된다.
- 기존 CLI 명령과 서버 빌드 및 테스트가 유지된다.
