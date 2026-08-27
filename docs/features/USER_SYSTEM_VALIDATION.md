# 사용자 시스템 수동 검증

## 1. 격리된 검증 환경 준비

운영 데이터 대신 임시 경로를 사용한다. 아래 파일을 `/tmp/govfs-user-test.toml`로 저장한다.

```toml
[server]
port = 3000

[server.logger]
path = "/tmp/govfs-user-test/logs/server.log"
accessLogPath = "/tmp/govfs-user-test/logs/access.log"
level = 0

[server.auth.admin]
username = "admin"
password = "admin-password"

[server.auth.jwt]
secret = "replace-with-a-long-random-test-secret"
exp = "24h"

[server.middlewares]
config = true
expvar = true
pprof = true
route = true
swagger = true

[server.webui]
enabled = true

[vfs.driver]
type = "badger"

[vfs.driver.badger]
path = "/tmp/govfs-user-test/drives"
gcInterval = "5m"
gcDiscardRatio = 0.7

[vfs.logger]
path = "/tmp/govfs-user-test/logs/vfs.log"
level = -1
```

서버를 실행한다.

```bash
go run ./cmd/govfs -config /tmp/govfs-user-test.toml
```

다른 터미널에서 헬스체크를 확인한다.

```bash
curl -i http://localhost:3000/healthz
```

## 2. 로그인과 관리자 권한

관리자 토큰을 발급한다. 이후 예시는 `jq`가 설치되어 있다고 가정한다.

```bash
ADMIN_TOKEN=$(curl -fsS http://localhost:3000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin-password"}' | jq -r .token)
```

일반 사용자를 생성하고 ID를 저장한다.

```bash
MEMBER=$(curl -fsS http://localhost:3000/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"username":"member","password":"member-password","role":"user"}')
MEMBER_ID=$(printf '%s' "$MEMBER" | jq -r .id)
```

일반 사용자로 로그인한다.

```bash
MEMBER_TOKEN=$(curl -fsS http://localhost:3000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"member","password":"member-password"}' | jq -r .token)
```

일반 사용자의 관리 API 호출이 `403`인지 확인한다.

```bash
curl -i http://localhost:3000/admin/users \
  -H "Authorization: Bearer $MEMBER_TOKEN"
```

## 3. 사용자 드라이브 격리

각 사용자 드라이브에 서로 다른 파일을 생성한다.

```bash
curl -i http://localhost:3000/vfs/ \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F 'name=admin.txt' -F 'file=@README.md'

curl -i http://localhost:3000/vfs/ \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -F 'name=member.txt' -F 'file=@README.md'
```

생성 응답은 `202 Accepted`이며 실제 저장은 비동기로 처리된다. 잠시 후 목록을 각각 조회한다.

```bash
curl -fsS 'http://localhost:3000/vfs/?q=/' -H "Authorization: Bearer $ADMIN_TOKEN" | jq
curl -fsS 'http://localhost:3000/vfs/?q=/' -H "Authorization: Bearer $MEMBER_TOKEN" | jq
```

관리자 목록에는 `admin.txt`만, 일반 사용자 목록에는 `member.txt`만 표시되어야 한다. 디스크에도 UUID별 디렉터리가 생성되어야 한다.

```bash
find /tmp/govfs-user-test/drives -maxdepth 1 -type d
```

## 4. SSE 온라인 상태와 관리자 화면

일반 사용자의 SSE 연결을 유지한다.

```bash
curl -N http://localhost:3000/sse/subscribe \
  -H "Authorization: Bearer $MEMBER_TOKEN"
```

브라우저에서 `http://localhost:3000`에 관리자로 로그인한다.

- 사이드바의 `ADMIN > Server status`로 이동해 시스템 DB의 items, 논리 사용량을 확인한다.
- `System DB details`에서 전체 key를 페이지 이동해 확인하고 user value에 `passwordHash`가 없는지 확인한다.
- `ADMIN > User management`로 이동해 `member` 상세를 연다.
- `member` 상세에서 Badger drive가 `Open`, SSE가 `Online (1)` 이상인지 확인한다.
- SSE용 `curl`을 종료한 뒤 상태 화면을 다시 열어 `Offline`으로 바뀌는지 확인한다.
- `config`, `routes`, `expvar`, `pprof` 링크가 열리는지 확인한다.

## 5. 이벤트와 계정 비활성화

관리자 화면의 사용자 상세에서 로그인과 VFS 변경 이벤트가 표시되는지 확인한다. 이벤트에는 사용자, 라우트 action, HTTP 상태와 시각만 있어야 하며 파일 경로나 요청 본문은 없어야 한다.

- `All activity`에서 전체 이벤트의 Previous/Next 페이지 이동을 확인한다.
- 사용자 상세에서는 해당 사용자의 이벤트만 표시되는지 확인한다.
- `Clear activity`는 확인창 이후 해당 사용자의 전체 이벤트만 삭제해야 한다.
- 이벤트 개별 생성·수정·삭제 UI나 API가 노출되지 않아야 한다.

일반 사용자를 비활성화한다.

```bash
curl -i -X PATCH "http://localhost:3000/admin/users/$MEMBER_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"disabled":true}'
```

기존 토큰이 즉시 `401`이 되는지 확인한다.

```bash
curl -i http://localhost:3000/auth/me \
  -H "Authorization: Bearer $MEMBER_TOKEN"
```

## 6. CLI 세션

서버가 실행 중인 상태에서 별도 임시 설정 경로로 로그인한다.

```bash
mkdir -p /tmp/govfs-cli-test
go run ./cmd/govfs-cli --config /tmp/govfs-cli-test login
```

프롬프트에 서버 URL, 사용자 이름과 비밀번호를 입력한 뒤 다음을 확인한다.

```bash
go run ./cmd/govfs-cli --config /tmp/govfs-cli-test ls /
sed -n '1,120p' /tmp/govfs-cli-test/.govfs/config
```

세션 파일에는 서버 URL, 사용자 이름, access token과 만료 시각만 있어야 하며 비밀번호가 없어야 한다.

## 7. 기존 단일 BadgerDB 이전

이 절차는 서버를 중지한 상태에서 수행한다. 먼저 기존 DB 전체를 별도 위치에 복사하고 복사본으로 복구 가능 여부를 확인한다.

1. 새 설정으로 서버를 한 번 시작해 최초 관리자만 생성한다.
2. `/admin/users`에서 최초 관리자 UUID를 확인한다.
3. 해당 관리자의 사용자 상세 상태 API나 VFS API를 호출하지 않고 서버를 중지한다.
4. `drives/{admin-uuid}`가 존재하지 않는지 확인한다. 존재하면 덮어쓰지 않는다.
5. 기존 BadgerDB 디렉터리 전체를 `drives/{admin-uuid}`로 복사한다. `.secret`도 반드시 포함한다.
6. 서버를 재시작하고 관리자 계정으로 목록, 파일 읽기, 새 파일 쓰기, backup과 restore를 확인한다.
7. 검증이 끝날 때까지 기존 DB와 별도 백업을 삭제하지 않는다.

예시 구조는 다음과 같다.

```text
/var/backups/govfs-before-users/       # 보존할 백업
~/.govfs/legacy-badger/                # 기존 DB
~/.govfs/drives/{admin-uuid}/          # 복사 대상
```

자동 이동이나 덮어쓰기는 제공하지 않는다. 대상 경로가 비어 있지 않거나 기존 DB 위치가 불확실하면 이전을 중단한다.

## 8. 최종 회귀 검사

수동 시나리오가 끝난 뒤 아래 명령을 직접 실행한다.

```bash
make audit
make test
make build
git diff --check
```

검증을 마치면 테스트 서버를 종료한다. `/tmp/govfs-user-test`와 `/tmp/govfs-cli-test`는 검증용 데이터이므로 내용을 확인한 뒤 정리한다.
