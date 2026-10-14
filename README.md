# govfs

Go로 작성된 다중 사용자 가상 파일 시스템(VFS)입니다. Fiber 서버가 사용자별
BadgerDB 또는 LocalStorage를 관리하고 REST API, SSE, Svelte 웹 UI와 CLI를
제공합니다.

## 주요 기능

- JWT 로그인과 관리자/일반 사용자 역할
- 사용자 UUID별로 격리된 BadgerDB 또는 LocalStorage 드라이브
- 파일 생성, 조회, 수정, 이동, 복사, 삭제와 Range Request
- 사용자 범위 SSE 알림
- 사용자 관리, 감사 이벤트, 서버·드라이브 상태 화면
- 사용자별 backup/restore와 Badger 암호화 키 rotation
- HTTP API를 사용하는 CLI와 MCP 서버

## Architecture

```text
CLI / Web UI
      │ HTTP + JWT / SSE
      ▼
Fiber Server
      ├── System DB: users, username index, audit events
      └── DriveManager
            └── drives/{user UUID}: selected VFS driver
```

서버의 `vfs.driver.type`은 `badger`와 `localstorage`를 지원합니다. 자세한 구조는
[DESIGN.md](./DESIGN.md), 선택 기준은 [BENCHMARK.md](./BENCHMARK.md)를
참조하세요. 암호화 키 rotation과 `/badger/*` 진단 API는 Badger 전용입니다.

Badger 기본 캐시 한도는 사용자 드라이브별 인덱스 16 MiB, 블록 32 MiB입니다.
큰 데이터의 반복 조회가 많은 환경에서는 `vfs.driver.badger.cacheSize`와
`blockCacheSize`를 높일 수 있습니다. MemTable 등 다른 DB 메모리는 이 한도에 포함되지 않습니다.

## Requirements

- Go 1.25+
- Yarn

Backend는 Fiber v3, Cobra, Viper, BadgerDB를 사용하고 Frontend는 Svelte 5,
Vite, TailwindCSS를 사용합니다.

## Install and Build

```bash
git clone https://github.com/shabatoily/govfs.git
cd govfs
make install
make build
```

특정 플랫폼만 빌드하려면 다음과 같이 지정합니다.

```bash
make build os=linux arch=amd64
```

빌드 결과는 `bin/govfs-{os}-{arch}`와 `bin/govfs-cli-{os}-{arch}`입니다.

배포 바이너리와 Docker 이미지는 디버그 심볼을 제거합니다. 서버는 Swagger UI를
제공하지 않습니다. `make swag`로 API 주석에서 문서를 생성할 수 있으며, 생성 결과는
`docs/swagger.json`, `docs/swagger.yaml`에 저장됩니다.

## Server Setup

서버는 `-config`를 생략하면 `~/.govfs/config.toml`을 읽습니다. 저장소의 예시
설정을 복사하고 최초 관리자와 JWT secret을 환경 변수로 제공합니다.

```bash
mkdir -p ~/.govfs
cp config.toml ~/.govfs/config.toml

export SERVER_AUTH_ADMIN_USERNAME=admin
export SERVER_AUTH_ADMIN_PASSWORD='change-this-password'
export SERVER_AUTH_JWT_SECRET='change-this-to-a-long-random-secret'

go run ./cmd/govfs
```

다른 설정 파일은 명시적으로 지정합니다.

```bash
go run ./cmd/govfs -config /path/to/config.toml
```

설치된 서버 바이너리는 사용자 서비스로 등록해 터미널과 분리하여 실행할 수 있습니다.
Linux는 systemd 사용자 서비스, macOS는 LaunchAgent를 사용하며 Windows에서는 관리자
권한으로 Windows Service를 등록합니다.

```bash
govfs service install
govfs service start
govfs service status
govfs service stop
govfs service restart
govfs service uninstall
```

다른 설정 파일을 서비스에서도 사용하려면 등록할 때 `-config`를 앞에 지정합니다.

```bash
govfs --config /path/to/config.toml service install
```

서비스 등록 시 현재 설정 파일의 절대 경로와 설정된 `SERVER_AUTH_ADMIN_USERNAME`,
`SERVER_AUTH_ADMIN_PASSWORD`, `SERVER_AUTH_JWT_SECRET` 환경 변수가 저장됩니다. 환경
변수를 변경한 경우 서비스를 제거한 뒤 다시 등록합니다.

`server.auth.admin`은 시스템 DB가 비어 있을 때 최초 관리자 한 명을 만드는
용도로만 사용됩니다. 이후 사용자 관리는 관리자 API 또는 웹 UI에서 수행합니다.
운영 환경에서는 재시작 후에도 기존 token을 검증할 수 있도록 JWT secret을 고정해야
합니다.

비밀번호 변경과 관리자 비밀번호 초기화는 기존 로그인 토큰을 무효화합니다.
세션 버전이 없는 이전 릴리즈의 토큰도 거부하므로 이 변경을 포함한 릴리즈로
업그레이드하면 웹 UI·CLI·MCP에서 다시 로그인해야 합니다.

LocalStorage의 부모 디렉터리는 파일 생성 전에 등록해야 합니다. `.vfs*` 이름은
내부 저장용으로 예약합니다. 변경 완료 이후에는 인덱스가 저장되며 미완료 작업은
재개방 시 복구됩니다. 기존 파일·인덱스의 경로 또는 크기가 불일치하면 개방을
거부하고 원본을 보존합니다. 데이터를 백업한 뒤 원인을 확인하고 복구해야 합니다.

키 회전은 실행 중인 DB 작업과 현재 읽기 호출이 끝날 때까지 기다립니다. 성공 후 새 키로 DB를
재개방하고 키 파일도 갱신하므로 서버 재시작 후에도 드라이브를 사용할 수 있습니다.

## 웹 UI 복사·이동

파일 트리의 Copy 또는 Move 버튼에서 이름을 포함한 전체 대상 경로를 입력합니다.
같은 경로의 항목이 있으면 이름을 바꾸거나 Replace를 선택할 수 있습니다.
폴더 Replace는 기존 하위 항목까지 교체하며, 되돌릴 수 없습니다.

WebUI는 기존 이동·복사 API에 `?wait=true`를 사용해 완료 결과(200) 또는 충돌(409)을
받습니다. WebUI는 `checkConflict: true`로 충돌 확인을 요청하며, 교체 시
`replaceId`에 확인한 대상 UUID를 전달합니다. 대상 UUID가 달라졌으면 다시 확인합니다.
서버 Move는 별도 조건이 없으면 기존 대상을 덮어씁니다. Copy는 기존처럼 충돌을
거부하며, `replaceId`로 교체 대상을 지정할 수 있습니다.
기존 CLI의 비동기 응답(202)과 SSE 방식은 유지합니다.

## Configuration

현재 `config.toml`의 전체 구조는 다음과 같습니다.

```toml
[server]
port = 3000

[server.logger]
path = "~/.govfs/logs/server.log"
accessLogPath = "~/.govfs/logs/access-log.log"
# Fiber log level: debug=-4, info=0, warn=4, error=8
level = 0

[server.auth]
[server.auth.admin]
username = "${SERVER_AUTH_ADMIN_USERNAME}"
password = "${SERVER_AUTH_ADMIN_PASSWORD}"

[server.auth.jwt]
secret = "${SERVER_AUTH_JWT_SECRET}"
exp = "24h"

[server.fiber]
# 최대 request body: 100 MiB
bodyLimit = 104857600

[server.middlewares]
# 운영 환경에서는 필요한 진단 endpoint만 활성화
config = false
envvar = false
expvar = false
pprof = false
route = false

[server.webui]
enabled = true

[vfs]
# 유휴 사용자 드라이브를 닫기까지의 시간. 0이면 비활성화
idleTimeout = "30m"

[vfs.driver]
# badger 또는 localstorage
type = "badger"

[vfs.driver.badger]
# 사용자 드라이브별 캐시 한도(MiB). 실제 메모리 점유량과는 다릅니다.
cacheSize = 16
blockCacheSize = 32
# 사용자 DB: {path}/{user UUID}
# 시스템 DB: {path의 상위 경로}/system/users
path = "~/.govfs/drives"
encryptKeyRotateDuration = "24h"
gcInterval = "5m"
gcDiscardRatio = 0.7

[vfs.driver.localstorage]
# type = "localstorage"일 때 사용자 파일 루트
path = "~/.govfs/drives"

[vfs.logger]
path = "~/.govfs/logs/vfs.log"
# VFS log level: trace=-1, debug=0, info=1, warn=2, error=3,
# fatal=4, panic=5, noLevel=6, disabled=7
level = -1
```

Viper는 설정 키의 점을 밑줄로 바꾼 환경 변수로 값을 덮어씁니다. 예를 들어
`server.auth.jwt.secret`은 `SERVER_AUTH_JWT_SECRET`으로 지정할 수 있습니다.
`~`로 시작하는 로그와 드라이버 경로는 실행 사용자의 홈 디렉터리로 확장됩니다.

사용자 드라이브 루트를 변경할 때는 기존 데이터 이전 절차를 먼저 확인하세요.
[USER_SYSTEM_VALIDATION.md](./docs/features/USER_SYSTEM_VALIDATION.md)에 수동 검증과
기존 단일 BadgerDB 이전 방법이 있습니다.

## CLI Usage

서버를 먼저 실행한 뒤 로그인합니다.

```bash
go run ./cmd/govfs-cli login
```

로그인 성공 시 서버 URL, 사용자 이름, access token과 만료 시각이 mode `0600`의
`~/.govfs/config`에 저장됩니다. 비밀번호는 저장되지 않습니다. Token이 없거나
만료되면 다시 `govfs login`을 실행해야 합니다.

주요 명령은 다음과 같습니다.

```text
login                         서버 로그인
info [-v]                     클라이언트/서버 정보
ls <path>                     파일 목록
tree <path>                   디렉터리 트리
stat <id>                     메타데이터
cp <src> <dst>                로컬-VFS 간 복사
mkdir <path>                  디렉터리 생성
rm <path>                     파일 또는 디렉터리 삭제
backup / restore / rotate     사용자 드라이브 유지보수
update [--check] [--server]    로컬 바이너리 업데이트
mcp                           현재 CLI 세션으로 MCP 서버 실행
```

관리자 계정으로 로그인한 뒤 `/admin` API 기능을 CLI에서도 사용할 수 있습니다.
응답은 VFS 명령과 같은 표 형식으로 출력되며 이벤트 삭제 성공 시 출력 없이 종료합니다.
서버 상태는 시스템 요약과 Badger 드라이브 표로 표시하고, 이벤트와 시스템 DB 조회는 페이지 정보도 표시합니다.
시스템 DB의 구조화된 value는 표 안에서 JSON으로 표시합니다.

```bash
govfs-cli admin status
govfs-cli admin system entries --page 1 --page-size 20
govfs-cli admin users list
govfs-cli admin users status <user-id>
govfs-cli admin users create <username> --password <password> --role user
govfs-cli admin users update <user-id> --role admin --disabled=false
govfs-cli admin users update <user-id> --password <new-password>
govfs-cli admin events --user-id <user-id> --page 1 --page-size 20
govfs-cli admin users clear-events <user-id>
```

`clear-events`는 해당 사용자의 감사 이벤트를 모두 삭제합니다.

CLI의 `--config`, `-c`는 세션 저장 기준 디렉터리를 지정합니다. 예를 들어
`--config /tmp/govfs-test`는 `/tmp/govfs-test/.govfs/config`을 사용합니다.

MCP의 `admin_status`, `admin_users`, `admin_user_status`, `admin_events`,
`admin_system_entries` 도구로 같은 관리자 조회 기능을 사용할 수 있습니다.
관리자 로그인 세션이 필요하며 관리자 생성·수정·삭제 도구는 제공하지 않습니다.
`admin_user_status`는 `id`(사용자 UUID)를 받습니다. 이벤트·시스템 DB 조회의
`page`, `page_size` 기본값은 각각 1, 20이며 페이지 크기는 1–100입니다.
`admin_events`는 선택적으로 `user_id`(사용자 UUID) 필터를 받습니다.

CLI의 `mkdir -p`와 재귀 업로드는 부모 생성 완료를 기다린 뒤 자식 작업을
진행합니다. 파일·백업 다운로드는 스트림을 임시 파일에 기록하고 성공한 결과만
교체합니다. MCP 변경 요청이 취소되거나 시간 초과되면 결과가 불확실하므로
변경 도구를 다시 호출하기 전에 MCP를 재연결하고 서버 상태를 확인하세요.

## API Groups

- `/auth`: 로그인, 로그아웃, 현재 사용자, 비밀번호 변경
- `/admin`: 사용자, 서버 상태, 시스템 DB, 감사 이벤트 관리
- `/vfs`: 사용자 파일과 backup/restore
- `/badger`: Badger 사용자의 드라이브 통계와 키 rotation
- `/sse`: `GET /sse/subscribe`, `POST /sse/publish/:id?`, client 목록

## 바이너리 업데이트

최초 설치 이후에는 쉘 스크립트 없이 공식 GitHub 릴리즈로 업데이트할 수 있습니다.

```bash
govfs-cli update --check   # 현재 CLI 버전과 최신 릴리즈 확인
govfs-cli update           # 실행 중인 CLI의 설치 파일 업데이트
govfs-cli update --server  # CLI와 같은 디렉터리의 로컬 govfs도 함께 업데이트
```

`--server`는 접속 대상인 원격 서버를 업데이트하지 않습니다. 심볼릭 링크로
실행했다면 실제 바이너리 경로를 갱신합니다. 설치 디렉터리에 쓰기 권한이
필요하며 `/usr/local/bin` 설치는 필요에 따라 `sudo govfs-cli update --server`로
실행합니다. 설정 파일, 로그인 세션 및 저장 데이터는 변경하지 않습니다.

업데이트는 하나의 릴리즈 버전을 고정하고 `checksums.txt`의 SHA-256을 검증한
후 바이너리를 교체합니다. 체크섬이 없는 이전 릴리즈는 업데이트 대상으로
사용할 수 없습니다. 서버와 CLI의 교체 중 오류가 발생하면 이전 바이너리로
복구를 시도하며, 복구 실패 시 원본 백업 경로를 오류에 표시합니다. 실행 중인
서버에는 재시작 이후 새 버전이 적용됩니다.

```bash
govfs service restart
```

Docker 배포는 컨테이너 이미지를 갱신해 재생성합니다. 업데이트 중 프로세스가
강제 종료되면 설치 디렉터리에 `.govfs-update.lock`이 남을 수 있습니다.
강제 종료나 전원 장애 시 두 바이너리의 교체가 모두 완료되지는 않을 수 있습니다.
다른 업데이트가 실행 중이지 않은지 확인한 뒤 잠금 디렉터리를 제거하고
업데이트를 다시 실행하세요.
체크섬은 다운로드 무결성을 검증하며 릴리즈 서명을 대신하지 않습니다.

## Docker

```bash
make build-docker tag=latest
docker run --rm -p 3000:3000 \
  --env-file .env \
  -v "$HOME/.govfs/config.toml:/etc/govfs/config.toml:ro" \
  -v "$HOME/.govfs:/home/govfs/.govfs" \
  ghcr.io/shabatoily/govfs:latest
```

Docker에서는 설정 파일과 선택한 드라이버의 `path`가 container 내부의 mount
경로와 일치해야 합니다. 환경별 실행 예시는 `docker-compose.yml`과
`docker-compose.dev.yml`을 참조하세요.
