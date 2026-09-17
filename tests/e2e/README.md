# 재실행 가능한 E2E 실행기

기존 Go `testing`, MCP SDK와 Python 표준 라이브러리를 사용한다. 제품 코드와 MCP 도구는 변경하지 않는다. 테스트마다 실제 서버·CLI를 빌드하고, Badger와 LocalStorage를 서로 다른 저장소에서 순차 검증한다.

## 코드 구성

- `e2e_test.go`: 실행 옵션, 빌드, 샘플 생성·재사용 및 드라이버별 실행
- `environment_test.go`: 격리 서버 수명 주기와 HTTP·MCP·CLI 호출 도우미
- `scenarios_test.go`: 파일, MCP, HTTP, CLI, 권한, 복구 순서의 검증 시나리오
- `fixtures.py`: 기존 `tools/gen` 출력에 형식별 샘플을 추가하고 manifest 생성
- `report.py`, `report_test.py`: Go 테스트 결과 집계와 보고서 생성·검증

시나리오는 같은 서버에서 순서대로 실행한다. 사례 이름은 보고서 식별자이므로 기능을 수정할 때도 유지한다. 범용 샘플 생성기는 `tools/gen`에, 이 테스트 전용 코드는 `tests/e2e`에 둔다.

## 실행

저장소 루트에서 실행한다. Go, Node/Yarn 및 설치된 WebUI 의존성, Python 3, FFmpeg(libx264/libvpx/libmp3lame), cwebp가 필요하다. `fixtures.py`는 기존 생성기의 PNG/MP4/TXT를 다른 실제 파일 형식과 함께 보완한다.

```sh
# 부모 디렉터리만 생성한다. -e2e-dir 대상은 아직 존재하지 않아야 한다.
mkdir -p /tmp/govfs-e2e
# 매번 새로운 이름을 사용한다.
go test -json ./tests/e2e -count=1 -timeout=15m -args \
  -e2e -e2e-dir /tmp/govfs-e2e/run-001 \
  > /tmp/govfs-e2e/run-001.jsonl

# 테스트가 실패해도 보고서는 생성한다.
python3 tests/e2e/report.py /tmp/govfs-e2e/run-001.jsonl /tmp/govfs-e2e/run-001
```

실패 시 `go test`는 0이 아닌 종료 코드를 반환한다. 보고서 생성 성공을 테스트 통과로 취급하지 않는다. 기본 `go test ./...`에서는 이 테스트를 건너뛴다. 실제 WebUI 빌드, 서버 빌드, fixture 검증이 끝나야 API 검증을 시작한다.

같은 샘플로 재현할 때:

```sh
go test -json ./tests/e2e -count=1 -timeout=15m -args \
  -e2e -e2e-dir /tmp/govfs-e2e/run-002 \
  -e2e-fixtures /tmp/govfs-e2e/run-001 \
  -e2e-drivers badger,localstorage \
  > /tmp/govfs-e2e/run-002.jsonl
python3 tests/e2e/report.py /tmp/govfs-e2e/run-002.jsonl /tmp/govfs-e2e/run-002
```

`-e2e-fixtures`는 `manifest.json`과 `fixtures/`가 함께 있는 최초 생성 디렉터리다. 재사용 실행에는 원본 위치가 `fixture-source.txt`로 기록된다. 원본 파일의 크기와 SHA-256이 다르면 실행을 중단한다. `-e2e-dir`를 생략하면 임시 디렉터리를 만들고 로그에 경로를 출력한다.

## 자동 검증 범위

- MCP 초기화·도구 목록, mkdir/upload/tree/stat/delete, 잘못된 입력과 10 MiB 경계값
- 41개 fixture의 메타데이터·다운로드 SHA-256·MIME
- HTTP 검색, 쓰기·코멘트·캐시, 정상/범위 밖 Range, 이동·복사·충돌 보호
- CLI ls/tree/stat/search/mkdir/cp/rm, 백업 파일명 및 백업/복원
- 관리자 조회, 사용자 생성·비활성화·재활성화, 권한과 사용자별 조회 격리
- Badger 전용 조회, 서버 재시작 후 데이터 유지·MCP 재접속
- 테스트 경로 삭제 및 생성한 서버·MCP 프로세스 종료

입력 오류 테스트는 MCP `isError`를 확인하며 transport 단절을 정상 거부로 간주하지 않는다. 한계값 실패를 기대값으로 바꿔 통과시키지 않는다. `202` 응답은 후속 조회로 반영을 확인한다. MCP 변경은 서버의 SSE 완료 대기를 통과해야 성공한다.

Badger의 복원은 버전을 보존하는 Load 동작이다. 같은 DB에 백업을 적재해도 더 최신 값은 유지되는 것으로 검증한다. 이 사례는 새 DB 복구나 증분 백업 체인 전체를 검증하지 않는다.

## 브라우저 및 추가 검증

이 실행기는 브라우저 자동화 도구를 설치하거나 실행하지 않는다. 브라우저 사례는 `SKIP`이며 보고서에서는 `BLOCKED`다. 미디어 실제 재생·PDF 표시·Overtype 저장·빠른 전환·화면 권한, SSE 사용자 간 이벤트 격리, 만료 토큰, 암호 키 교체와 이전 DB 마이그레이션 등은 [전수 테스트 지침](../../docs/testing/AGENT_E2E.md)에 따라 에이전트가 추가 실행한다. 자동 테스트만으로 전수 통과를 선언하지 않는다.

## 격리와 산출물

서버는 비어 있는 작업 디렉터리에서 시작하여 저장소 `.env`를 읽지 않는다. `SERVER_*`, `VFS_*` 환경변수도 서버/MCP 프로세스에서 제외한다. OS가 할당한 빈 포트를 사용하되, 현재 서버는 전체 인터페이스에 바인딩하므로 격리된 로컬 환경에서 실행한다.

실행 디렉터리에 기준 커밋·작업 트리 상태·fixture manifest·파일 ID·서버/MCP 로그가 남는다. 종료 시 서버와 MCP를 닫고 테스트 VFS 경로를 삭제한다. 오류 원인 분석을 위해 저장소·백업·fixture는 보존한다. 외부에서 테스트 프로세스를 강제 종료하면 정리가 실행되지 않을 수 있으므로 해당 실행의 서버 PID를 확인한다.

서버 설정과 CLI 세션에는 임시 비밀번호/JWT가 들어 있다. 파일 권한은 0600이며, 실행 디렉터리 전체를 공유하지 않는다. 공유할 때는 `report.md`, `results.json`, 비밀을 제외한 로그만 선택한다. `make audit`, 전체 Go/WebUI 단위 테스트·타입 검사는 이 실행기와 별도로 수행한다.
