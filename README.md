# go-vfs

Go로 작성된 가상 파일 시스템 (VFS) 프로젝트입니다. 웹 서버를 통해 파일 시스템에 접근하고 관리할 수 있는 기능을 제공하며, 웹 UI를 통해 사용자 친화적인 인터페이스를 제공합니다.

## 기능

- **가상 파일 시스템 (VFS)**: 플러그형 드라이버 아키텍처를 지원하여 **BadgerDB** 또는 **LocalStorage**를 스토리지 백엔드로 선택할 수 있습니다.
- **웹 서버**: RESTful API를 통해 VFS에 접근할 수 있는 엔드포인트를 제공합니다.
- **실시간 알림 (SSE)**: Server-Sent Events를 통해 파일 시스템 변경 사항이나 작업 상태를 실시간으로 클라이언트에 전달합니다.
- **웹 UI**: Svelte 5 기반의 모던 웹 인터페이스를 통해 VFS를 시각적으로 탐색하고 조작할 수 있습니다.
- **CLI**: 파일 시스템 관리 및 데이터 조작을 위한 다양한 유틸리티 명령어를 제공합니다.
- **설정 관리**: `config.toml` 파일을 통해 애플리케이션의 동작을 유연하게 변경할 수 있습니다.
- **백업 및 복원**: VFS 데이터의 백업 및 복원, 암호화 키 로테이션 기능을 지원합니다.

## 아키텍처 (Client-Server)

go-vfs는 Client-Server 아키텍처를 따릅니다.

- **Server (Daemon)**: 서버 바이너리(예: `go-vfs`)로 실행되며, 스토리지 엔진(BadgerDB/LocalStorage)을 관리하고 API를 제공하는 백그라운드 프로세스입니다.
- **Client (CLI)**: CLI 바이너리(예: `go-vfs-cli`)를 사용하여 실행 중인 서버에 HTTP API 요청을 보내 작업을 수행합니다.

> **Note**: 따라서 CLI 명령어를 사용하기 위해서는 먼저 서버 프로세스를 실행해야 합니다.

## 드라이버 선택 가이드

사용 목적에 따라 적절한 드라이버를 `config.toml`에서 선택할 수 있습니다.

| 드라이버 | 특징 | 권장 시나리오 |
| :--- | :--- | :--- |
| **LocalStorage** | **최고의 성능**. OS 파일 시스템 직접 사용. 파일 직접 접근 가능. | 고성능 비디오 스트리밍, 대용량 파일 서비스, 개발 및 디버깅. |
| **BadgerDB** | **높은 보안 및 이식성**. 단일 DB 파일에 모든 데이터 및 메타데이터 저장. 암호화 지원. | 보안이 중요한 데이터, 간편한 배포 및 백업이 필요한 경우. |

자세한 성능 비교는 [BENCHMARK.md](./BENCHMARK.md)를 참조하세요.

## 개발 환경

- **Backend**: Go 1.25+
  - **Web Framework**: [Fiber](https://github.com/gofiber/fiber)
  - **CLI**: [Cobra](https://github.com/spf13/cobra)
  - **Config**: [Viper](https://github.com/spf13/viper)
  - **Storage**: [BadgerDB](https://github.com/dgraph-io/badger)
- **Frontend**: Svelte 5, Vite, TailwindCSS
  - **Package Manager**: Yarn

## 설치 방법

```bash
git clone https://github.com/meteormin/go-vfs.git
cd go-vfs
go mod tidy
cd webui && yarn install
```

## 빌드 방법

### Makefile

Makefile을 사용하여 간단하게 빌드할 수 있습니다.

**기본 빌드:**

```bash
make build
```

**특정 OS/Arch 빌드:**

```bash
make build os=linux arch=amd64
```

### Docker

Docker를 사용하여 빌드하고 실행할 수 있습니다.

**Docker 이미지 빌드:**

```bash
make build-docker tag=latest
```

**Docker 컨테이너 실행:**

```bash
docker run -d -p 3000:3000 --name go-vfs go-vfs:latest
```

## SSE (Server-Sent Events) API

애플리케이션은 실시간 상태 업데이트를 위해 SSE를 지원합니다.

- **Subscribe**: `GET /sse/subscribe`
  - 클라이언트가 이벤트를 수신하기 위해 연결하는 엔드포인트입니다.
- **Publish**: `POST /sse/:id/publish`
  - 특정 클라이언트(`:id`) 또는 모든 클라이언트에게 이벤트를 발송합니다.


## CLI 명령어

애플리케이션은 다음과 같은 주요 명령어를 제공합니다.

### 서버 실행 (Server Execution)

서버는 별도의 바이너리로 제공되며, 데몬 형태 또는 백그라운드 프로세스로 실행해야 합니다.

```bash
# 직접 실행 (예시: Linux/AMD64)
./go-vfs-linux-amd64

# 또는 systemd 서비스 등으로 등록하여 실행
```

### CLI 명령어 (Client)

CLI 바이너리(`go-vfs-cli-***`)를 사용하여 실행 중인 서버를 제어합니다. 아래 예시는 `go-vfs-cli`로 통칭합니다.

#### 기본 명령어 (Root Level)

- **데이터 관리**
  - `go-vfs-cli backup`: VFS 데이터베이스 백업
  - `go-vfs-cli restore`: 백업 파일에서 복원
  - `go-vfs-cli rotate`: 암호화 키 교체

- **파일 조작**
  - `go-vfs-cli ls`: 파일 목록 조회
  - `go-vfs-cli tree`: 트리 구조 조회
  - `go-vfs-cli stat`: 파일 메타데이터 조회
  - `go-vfs-cli cp`: 로컬-VFS 간 파일 복사
  - `go-vfs-cli mkdir`: 디렉토리 생성
  - `go-vfs-cli rm`: 파일/디렉토리 삭제

#### 클라우드 명령어 (Cloud)

클라우드 스토리지 관련 명령어는 `cloud` 서브커맨드 하위에 있습니다.

- `go-vfs-cli cloud list`: 클라우드 파일 목록 조회
- `go-vfs-cli cloud upload`: 로컬 파일을 클라우드로 업로드
- `go-vfs-cli cloud download`: 클라우드 파일을 로컬로 다운로드

## 설정 (config.toml)

`config.toml` 파일을 통해 애플리케이션의 설정을 변경할 수 있습니다.

```toml
[server]
# 웹 서버 포트
port = 3000

[server.config]
# 라우트 정보 출력 여부
enablePrintRoutes = false

[server.logger]
path = "./logs/server.log"
accessLogPath = "./logs/access-log.log"
# log-level: debug=-4, info=0, warn=4, error=8
level = 0

[server.basicAuth]
enabled = false
username = "${BASIC_AUTH_USERNAME}"
password = "${BASIC_AUTH_PASSWORD}"

[vfs.logger]
path = "./logs/vfs.log"
# log-level: debug=-4, info=0, warn=4, error=8
level = 8

[vfs.driver]
# 사용할 드라이버 타입: "badger" 또는 "localstorage"
type = "badger"

[vfs.driver.badger]
path = "./data"
encryptKeyRotateDuration = "24h"

[vfs.driver.localstorage]
path = "./vfs_root"

[cloud.googleDrive]
clientID = "${CLOUD_GOOGLEDRIVE_CLIENT_ID}"
clientSecret = "${CLOUD_GOOGLEDRIVE_CLIENT_SECRET}"
parentFolderID = "go-vfs"
```
