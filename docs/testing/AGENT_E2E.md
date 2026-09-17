# 에이전트 기반 전수 테스트 실행 지침

이 문서는 생성기, 기존 MCP 도구, HTTP/CLI, 브라우저를 조합하여 govfs를 반복 검증하는 실행 계약이다. MCP 기능 추가는 요구하지 않는다. 문서 작성만으로 테스트가 수행되거나 통과한 것은 아니다.

## 1. 범위와 원칙

- 전수 테스트는 아래 행렬의 모든 적용 가능한 사례를 Badger와 LocalStorage에서 실행하는 것을 의미한다. 무한한 파일 형식·코덱 조합이나 부하 테스트를 의미하지 않는다.
- 현재 코드가 기준이다. 실행 전에 라우트·MIME·CLI·WebUI 기능을 다시 읽고 새 기능이 있으면 행렬에 추가한다. 구현에서 발견한 버그를 기대 결과로 바꾸지 않는다.
- 기존 데이터와 분리된 서버, 저장소, 계정만 사용한다. 테스트 계정의 생성·수정·삭제·복원까지 실행 요청에 포함한다.
- 실행은 순차 진행한다. 특히 MCP 변경 도구는 프로세스 내부에서 직렬 실행되며 SSE 완료 대기 제한은 30초다.
- 실패는 기록하고 독립적인 사례를 계속 실행한다. 인증·서버·도구 연결 문제로 실행하지 못한 사례를 통과로 처리하지 않는다.
- 소스 수정과 커밋은 테스트 실행과 별개다. 수정이 요청되지 않았다면 결함과 재현 방법만 보고한다.

## 2. 코드 근거와 도구 역할

| 대상 | 근거 | 역할·제한 |
| --- | --- | --- |
| 생성기 | [이미지](../../tools/gen/image/main.go), [영상](../../tools/gen/video/main.go), [텍스트](../../tools/gen/text/main.go) | PNG, H.264/yuv420p MP4, ASCII TXT 생성 |
| MCP | [도구](../../internal/mcp/tools.go), [서버](../../internal/mcp/server.go) | `vfs_tree`, `vfs_stat`, `vfs_mkdir`, `vfs_upload`, `vfs_delete` |
| API | [라우트](../../internal/server/init.go), [핸들러](../../internal/server/handlers/vfs.go), [클라이언트](../../internal/client/vfs.go) | 다운로드·쓰기·검색·이동·복사·코멘트·백업/복원 |
| 검색 | [서비스](../../internal/server/services/vfs.go) | `/vfs/search?q=...`: 이름의 부분 문자열, 대소문자 무시. MCP에는 검색 입력이 없음 |
| CLI | [설정/로그인](../../internal/cli/root.go), [명령](../../internal/cli/vfs/commands.go) | 기존 사용자 세션 재사용, 검색·전송·백업 등 검증 |
| MIME | [상수](../../constants.go), [매핑](../../vfs.go) | 확장자별 기대 HTTP Content-Type |
| WebUI | [분기](../../webui/src/App.svelte), [판별](../../webui/src/lib/utils.ts), [읽기](../../webui/src/lib/vfs.ts) | 미디어/PDF 미리보기, 나머지는 Overtype |
| 사용자/관리자 | [기존 검증 지침](../features/USER_SYSTEM_VALIDATION.md) | 로그인·사용자 격리·권한·관리자 화면 검증을 함께 실행 |

`vfs_tree` 결과에서 에이전트가 이름을 찾는 것은 서버 검색 테스트가 아니다. 검색은 HTTP API와 CLI `search`로 검증한다. HTTP 성공으로 MCP 또는 브라우저 검증을 대신하지 않는다.

## 3. 실행 입력과 산출물

실행 요청에 다음 값을 명시한다. 생략 시 아래 기본값을 사용하되 실제 선택값을 보고서에 남긴다.

| 입력 | 기본값 |
| --- | --- |
| 대상 | 현재 체크아웃. 커밋 ID와 미커밋 변경 상태 기록 |
| 드라이버 | `badger`, `localstorage` 순차 실행 |
| 포트 | 3300. 사용 중이면 빈 포트 선택 후 모든 설정에 반영 |
| 작업 디렉터리 | `/tmp/govfs-e2e/<run-id>`; 실행마다 새 ID |
| VFS 작업 경로 | `/e2e-<run-id>` |
| 규모 | 일반 샘플 각 3개 이하, 영상 2초. MCP 경계값 파일은 별도 생성 |
| 브라우저 | 사용할 브라우저 이름·버전을 기록 |
| 정리 | 생성한 VFS 데이터와 프로세스만 정리. 보고서·실패 증거는 보존 |

작업 디렉터리에는 `fixtures/`, `bin/`, `badger/`, `localstorage/`, `evidence/`, `manifest.json`, `results.json`, `report.md`를 둔다. 드라이버별 `drives/`, `logs/`, `client/`를 분리한다.

`manifest.json`에는 파일별 상대 경로, 바이트 수, SHA-256, 예상 MIME, 예상 화면(editor/image/video/audio/pdf)을 기록한다. UUID는 매 실행 새로 얻으며 이전 실행 값을 재사용하지 않는다. 랜덤 생성 결과를 보존하면 같은 입력으로 재현할 수 있다.

## 4. 코드화된 실행기와 사전 점검

[tests/e2e](../../tests/e2e/README.md)의 실행기를 먼저 사용한다. 실행 명령, fixture 재사용, JSON/Markdown 보고서 생성 방법은 해당 README를 따른다. 자동 범위에 없는 브라우저·상세 SSE·관리자 사례는 아래 행렬에 따라 추가 실행한다. 실행기의 SKIP은 전수 테스트 보고서에서 BLOCKED로 남긴다.

1. `git status --short --branch`, `git rev-parse HEAD`로 기준 상태를 기록한다. 사용자 변경을 지우거나 자동 커밋하지 않는다.
2. Go 버전은 `go.mod`, Node/Yarn은 프로젝트 환경을 따른다. FFmpeg의 `libx264` 지원 및 브라우저 제어 도구 사용 가능 여부를 확인한다.
3. 아래 검사를 실행하고 종료 코드와 로그를 보존한다. 필요한 의존성이 없으면 설치 필요 상태를 기록한다.

   ```sh
   go test ./...
   go build ./...
   yarn --cwd webui test run
   yarn --cwd webui run check
   make audit
   ```

4. 기존 취약점 등 독립적인 검사 실패는 보고하고 E2E를 계속할 수 있다. 빌드 실패는 해당 실행을 BLOCKED로 처리한다.
5. 생성기 및 MCP의 알려진 점검 항목을 다시 확인한다. 아래는 작성 당시 코드 관찰이며 수정되었으면 새 결과로 갱신한다.
   - 텍스트 생성기의 기본 크기와 내용은 랜덤이다. 크기를 명시하고 실제 파일 수·크기를 확인한다.
   - 생성기들은 개별 파일 생성 오류를 출력해도 성공 종료할 수 있다. 종료 코드만으로 통과시키지 않는다.
   - MCP의 `DecodedLen` 사전 검사가 Base64 패딩을 포함한 상한을 사용하여 정확히 10 MiB인 파일을 거부할 수 있다. 한계값 검증에서 결함 여부를 확인한다.

## 5. 격리 서버 준비

아래 `RUN_DIR`는 실제 절대 경로로 치환한다. 셸 작업은 저장소 루트에서 수행한다.

```sh
# 예시: 각 실행마다 다른 경로를 지정한다.
RUN_DIR=/tmp/govfs-e2e/run-001
mkdir -p "$RUN_DIR/bin" "$RUN_DIR/fixtures" "$RUN_DIR/evidence"
mkdir -p "$RUN_DIR/badger/logs" "$RUN_DIR/badger/client"
mkdir -p "$RUN_DIR/localstorage/logs" "$RUN_DIR/localstorage/client"
yarn --cwd webui build
go build -o "$RUN_DIR/bin/govfs-server" ./cmd/govfs
go build -o "$RUN_DIR/bin/govfs-cli" ./cmd/govfs-cli
```

서버는 WebUI 산출물을 임베드하므로 WebUI를 먼저 빌드한다. 루트 [config.toml](../../config.toml)을 실행 디렉터리에 복사하고 다음을 변경한다.

| 설정 | Badger 실행 예시 |
| --- | --- |
| `server.port` | `3300` |
| `server.logger.path` | `RUN_DIR/badger/logs/server.log` |
| `server.logger.accessLogPath` | `RUN_DIR/badger/logs/access.log` |
| `server.webui.enabled` | `true` |
| `vfs.driver.type` | `badger` |
| `vfs.driver.badger.path` | `RUN_DIR/badger/drives` |
| `vfs.driver.localstorage.path` | `RUN_DIR/localstorage/drives` |
| `vfs.logger.path` | `RUN_DIR/badger/logs/vfs.log` |

경로는 TOML 저장 전에 실제 절대 경로로 치환한다. LocalStorage 실행은 드라이버와 로그 경로를 바꾼 별도 설정을 사용한다. 사용자 DB는 선택한 `drives`의 부모 아래 `system/users`에 생성되므로 부모 경로까지 분리해야 한다.

`SERVER_AUTH_ADMIN_USERNAME`, `SERVER_AUTH_ADMIN_PASSWORD`, `SERVER_AUTH_JWT_SECRET`에 테스트 전용 값을 제공한다. 토큰·비밀번호는 보고서·명령 출력에 남기지 않는다. 서버는 현재 포트 전체 인터페이스에 바인딩하므로 노출되지 않는 로컬 환경에서 실행한다.

```sh
"$RUN_DIR/bin/govfs-server" --config "$RUN_DIR/badger/server.toml"
```

서버 작업 디렉터리는 저장소 밖의 실행 디렉터리로 설정하여 프로젝트 `.env`가 테스트 설정을 덮어쓰지 않게 한다. 서버는 별도 터미널 또는 관리 가능한 프로세스로 유지하고 PID와 로그를 기록한다. `/healthz` 응답과 로그인 성공을 확인한 뒤 진행한다. 같은 저장소를 여러 서버에서 동시에 열지 않는다.

## 6. 로그인과 MCP 연결

```sh
"$RUN_DIR/bin/govfs-cli" --config "$RUN_DIR/badger/client" login
```

대화형 터미널에서 테스트 서버 주소와 계정을 입력한다. 현재 CLI는 `--config` 값 아래 `.govfs/config`를 사용하므로 파일 경로나 `.govfs` 자체를 넘기지 않는다. 로그인은 MCP가 시작되기 전에 끝내며, 만료되면 재로그인하고 MCP를 다시 연결한다.

Codex 설정의 예시다. 실제 실행 경로로 치환하고 LocalStorage 차례에는 해당 `client` 경로를 사용한다.

```toml
[mcp_servers.govfs_test]
command = "/tmp/govfs-e2e/run-001/bin/govfs-cli"
args = ["--config", "/tmp/govfs-e2e/run-001/badger/client", "mcp"]
startup_timeout_sec = 30
tool_timeout_sec = 60
```

설정 형식: [공식 MCP 문서](https://learn.chatgpt.com/docs/extend/mcp?surface=cli). 이 서버는 stdio 방식이며 `/mcp` HTTP 주소를 등록하는 방식이 아니다. Codex 타임아웃과 내부 SSE 30초 제한은 별개다.

연결 후 도구 목록에 5개 도구가 있는지 확인하고 `vfs_tree`를 호출한다. 에이전트 도구가 노출되지 않으면 직접 호출 사례는 BLOCKED지만 아래 SDK 실행기로 프로토콜 검증은 계속할 수 있다. 연결 불가를 이유로 HTTP 호출을 MCP 성공으로 보고하지 않는다.

대량 Base64를 대화에 출력하지 않는다. [Go 실행기](../../tests/e2e/e2e_test.go)는 기존 MCP SDK로 로컬 파일을 인코딩하여 실제 stdio 서버의 `CallTool`로 보낸다. Codex 도구 등록 없이도 MCP 프로토콜 검증이 가능하다. 에이전트 직접 호출 검증과 SDK 실행기 검증을 구분해 기록한다. 서버 도구 추가는 하지 않는다.

## 7. 샘플 생성과 기대값

```sh
go run ./tools/gen/image -count 3 -width 640 -height 480 -out "$RUN_DIR/fixtures"
go run ./tools/gen/video -count 2 -duration 2 -width 640 -height 480 -out "$RUN_DIR/fixtures"
go run ./tools/gen/text -count 3 -bytes 4096 -out "$RUN_DIR/fixtures"
```

파일 개수, 실제 바이트 수, PNG 디코딩, FFprobe 결과로 생성 성공을 확인한다. 기존 파일을 재사용하는 재현 실행에서는 다시 생성하지 않고 manifest의 해시와 비교한다.

추가 샘플은 다음 표를 따른다. 미디어는 실제 형식에 맞는 파일을 생성·변환한다. PNG의 확장자만 바꾸어 JPEG 지원을 검증하지 않는다.

| 분류 | 샘플 |
| --- | --- |
| 이미지 | MIME 매핑에 있는 PNG/JPEG/WebP/GIF/SVG, 대문자 확장자 |
| 영상 | MP4/WebM/AVI/MOV/MKV/MPEG, 매핑에 있는 별칭 확장자 |
| 오디오/PDF | MP3, WAV, 정상 PDF 한 페이지 |
| 에디터 | TXT/MD/JSON/XML/HTML/CSS/JS/MJS/CSV/TS/Go/YAML, 무확장자, 알 수 없는 확장자 |
| 텍스트 내용 | 빈 파일, 한글·이모지, LF/CRLF, 긴 줄, 4 KiB 및 1 MiB |
| 파일명/경로 | 공백, 한글, 중첩 경로, 동일 이름의 다른 디렉터리 |
| 바이너리 기본 분기 | ZIP 등: 에디터 분기만 확인. 텍스트로 저장해 원본이 유지된다고 기대하지 않음 |
| MCP 경계 | 0, 10 MiB−2, 10 MiB−1, 10 MiB, 10 MiB+1 바이트; 잘못된 Base64 |

정상 미디어를 준비하지 못하면 해당 형식은 BLOCKED다. 본문에 있는 목록과 현재 `Meta.MIME()`/`inferType()`을 대조하여 누락된 확장자를 추가한다. 10 MiB 이하 정상 파일은 MCP에서 수락하고 초과는 거부하는 것이 기대값이다. HTTP 업로드 한도는 별도 설정이며 MCP 한도와 혼동하지 않는다.

## 8. 실행 행렬

각 행은 드라이버와 개별 입력별 사례로 펼쳐 별도 결과를 기록한다.

| ID | 채널 | 절차 및 통과 기준 |
| --- | --- | --- |
| MCP-01 | MCP | 초기화·도구 5개 및 입력 스키마 확인, 로그인 실패/만료 시 오류 확인 |
| MCP-02 | MCP | 실행 전용 디렉터리 생성 → 파일 업로드 → tree/stat의 UUID·경로·크기 일치 |
| MCP-03 | MCP+HTTP | 모든 업로드 파일 다운로드 후 원본 크기·SHA-256 일치 |
| MCP-04 | MCP | 상대/빈 경로, 잘못된 UUID/Base64, 중복 경로, 크기 경계 입력. 오류와 불필요한 생성 부재 확인 |
| MCP-05 | MCP | 파일·디렉터리 삭제 완료 후 tree와 stat에서 부재 확인. 반복 삭제 결과도 기록 |
| API-01 | HTTP | 목록·트리·stat 일치, 파일 읽기의 Content-Type·크기·원본 바이트 확인 |
| API-02 | HTTP | 텍스트 쓰기·코멘트 변경 후 재조회. octet-stream은 장기 캐시하지 않는지 확인 |
| API-03 | HTTP | 이동·복사 정상/충돌/덮어쓰기 보호, 원본 유지·제거 및 대상 해시 확인 |
| API-04 | HTTP+CLI | 이름 부분 검색, 대소문자·한글·공백·불일치·빈 검색어. root 제외 및 사용자 범위 확인 |
| API-05 | HTTP | Range 정상 요청의 206·Content-Range·바이트 일치, 범위 밖 요청 오류 확인 |
| API-06 | HTTP/CLI | 백업 → 데이터 변경 → 복원 → 경로·내용 검증. Badger는 같은 DB에 Load 시 더 최신 버전이 유지되는 것이 정상이며 시점 되돌리기를 기대하지 않는다. 같은 계정의 격리된 저장소에서 수행 |
| CLI-01 | CLI | ls/tree/stat/search/cp/mkdir/rm 및 backup/restore를 현재 --help에 따라 실제 호출 |
| SSE-01 | MCP/HTTP/UI | 변경 완료 이벤트와 최종 상태 일치, 다른 사용자의 이벤트가 전달되지 않음 |
| UI-01 | 브라우저 | 로그인 → 업로드 → 목록/검색/선택. 콘솔·요청 실패 수집 |
| UI-02 | 브라우저 | 각 이미지 실제 디코딩, 영상·오디오 재생 시간 진행/탐색, PDF 본문 표시 |
| UI-03 | 브라우저 | 모든 에디터 대상에서 내용 로드 → 수정 → 저장 → 다른 파일 선택 → 재열기/새로고침 후 내용 확인 |
| UI-04 | 브라우저 | 빠른 파일 전환 후 잘못된 파일에 내용·저장이 적용되지 않음, 코멘트 저장 확인 |
| AUTH-01 | HTTP/UI | 관리자와 일반 사용자 A/B 준비. 미인증·비활성·만료·역할 제한, 사용자 데이터 격리 검증 |
| ADMIN-01 | HTTP/UI | 사용자 생성/변경, 상태·이벤트·시스템 조회 등 기존 사용자 검증 문서 전체 수행 |
| DRIVER-01 | HTTP/CLI | Badger keys/stats/rotate 검증. LocalStorage에서는 지원 여부/거부 응답 확인 |
| RECOVERY-01 | 서버+MCP | 정상 종료·재시작 뒤 파일/메타데이터 유지, MCP 재연결 뒤 변경 작업 성공 |

현재 라우트 목록과 행렬을 비교하고 모든 등록 작업이 적어도 한 사례에 대응하도록 기록한다. 관리·보안 사례의 상세 절차는 [사용자 시스템 검증](../features/USER_SYSTEM_VALIDATION.md)을 따르되 서버 주소·저장소를 본 실행 값으로 바꾼다. 서비스 설치/제거 및 운영체제 서비스 관리, 스트레스/성능 측정은 이 전수 테스트 범위 밖이다.

### 비동기 작업과 판정

- HTTP `202 Accepted`는 완료가 아니다. SSE 최종 이벤트와 후속 조회를 확인한다. `wait=true`는 이를 구현한 이동·복사에만 사용한다.
- 일반 상태 확인은 최대 30초 동안 간격을 두어 조회한다. 고정 sleep 후 성공을 가정하지 않는다.
- MCP 결과는 transport 오류뿐 아니라 tool 결과의 `isError`도 확인한다.
- 변경 요청이 timeout이면 먼저 tree/stat으로 실제 반영 여부를 확인한다. 무조건 재전송하여 중복 생성하지 않는다.
- 미디어 요소 존재만으로 렌더링 성공이 아니다. 디코딩·재생을 확인한다. 정상 파일인데 브라우저 코덱 미지원이면 `UNSUPPORTED`로 기록하고 브라우저/코덱 근거를 남긴다.
- 저장소와 MIME 처리는 통과했어도 UI 재생 실패는 별도 결과로 남긴다.

## 9. 보고서와 재실행

각 사례는 다음 형식으로 `results.json`에 기록한다.

```json
{
  "case_id": "MCP-03/png",
  "driver": "badger",
  "status": "PASS",
  "expected": "다운로드 SHA-256이 manifest와 일치",
  "actual": "일치",
  "evidence": ["evidence/badger/MCP-03-png.json"],
  "duration_ms": 120,
  "attempt": 1
}
```

상태는 `PASS`, `FAIL`, `BLOCKED`, `UNSUPPORTED`만 사용한다. `UNSUPPORTED`는 명시된 제품/환경 지원 한계에만 사용하며 결함을 숨기는 용도로 쓰지 않는다. 미실행 사례에는 원인을 적어 BLOCKED로 남긴다.

`report.md`에는 기준 커밋/변경 파일, 도구 버전, 서버 설정(비밀 제외), fixture manifest, 채널별·드라이버별 건수, 실패 재현 단계, 로그/스크린샷 경로, 정리 결과를 담는다. 모든 적용 사례가 PASS이고 미해결 FAIL/BLOCKED가 없어야 전수 통과다. UNSUPPORTED가 있으면 제외 범위가 있는 결과라고 명시한다.

재실행 방식:

1. 같은 manifest와 fixture를 사용하고 새로운 run-id·저장소·UUID를 만든다.
2. 실패 사례만 재현할 때도 로그인·데이터 생성 등 선행 단계를 실행한다. 이전 결과는 덮어쓰지 않는다.
3. 코드 수정 후 해당 사례를 재검증하고 최종 전수 실행을 별도로 수행한다.
4. 실행 도중 중단되면 완료된 사례와 남은 사례를 보고서에 저장한다. 재개 시 서버/계정/데이터 상태를 먼저 확인한다.
5. 종료 시 테스트용 파일을 삭제하고 생성한 서버·MCP 프로세스만 종료한다. 실패 증거와 fixture는 보존하며 토큰 파일은 공유 산출물에서 제외한다.

## 10. 에이전트에 전달할 실행 지시

아래를 새 작업에 전달한다. 이 문서를 읽는 것만으로 실행을 시작하지 않으며, 사용자가 테스트 실행을 요청했을 때 적용한다.

```text
현재 저장소의 docs/testing/AGENT_E2E.md를 읽고 전수 테스트를 실행하세요.
먼저 tests/e2e/README.md의 기존 실행기로 자동 범위를 검증하고,
결과와 별도로 브라우저 및 나머지 수동 사례를 실행하세요.

범위는 Badger와 LocalStorage, 기존 MCP 5개 도구, HTTP/CLI,
브라우저 WebUI, 사용자/관리자 기능입니다. MCP 기능은 추가하지 마세요.

테스트 전용 서버·계정·저장소를 준비하고, 그 범위 안의 데이터 생성,
수정, 삭제, 백업/복원, 서버 재시작을 수행하세요. 운영 데이터는 사용하지 마세요.
실행마다 새 /tmp/govfs-e2e/<run-id>와 VFS 작업 경로를 사용하세요.
현재 커밋과 미커밋 변경을 기록하고 기존 사용자 파일을 변경하지 마세요.

생성기와 실제 형식 샘플을 활용하고 manifest에 크기·해시·기대 MIME을 남기세요.
필요한 최소 테스트 보조 실행기는 작성할 수 있지만 제품 코드는 수정하지 마세요.
큰 Base64는 대화에 출력하지 말고 MCP SDK 실행기에서 직접 전송하세요.
MCP 미지원 기능은 HTTP/CLI로, 렌더링은 실제 브라우저로 검증하세요.
검색은 MCP tree 필터링으로 대체하지 마세요.

도구·인증·브라우저가 부족하면 해당 항목을 BLOCKED로 기록하고
독립적으로 실행 가능한 검증을 계속하세요. 실패를 임의로 통과시키지 마세요.
results.json, report.md, 재현 절차, 로그·스크린샷을 저장하고
최종 답변에 실제 절대 경로 링크와 통과/실패/미실행/미지원 건수를 알려주세요.
테스트 생성 데이터와 실행한 프로세스를 정리하되 실패 증거는 보존하세요.
```
