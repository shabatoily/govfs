# govfs 프로젝트 디자인 문서

## 1. 프로젝트 개요

**govfs**는 Go 언어로 작성된 가상 파일 시스템(Virtual File System) 프로젝트입니다. BadgerDB를 기반으로 한 로컬 스토리지와 Google Drive와 같은 클라우드 스토리지 통합을 지원합니다. 웹 서버, 웹 UI, 그리고 강력한 CLI 도구를 통해 파일 시스템을 효율적으로 관리할 수 있습니다.

## 2. 프로젝트 구조

```text
/
├── bootstrap/         # 애플리케이션 초기화 로직 (VFS, Server)
├── cli/               # CLI 명령어 구현체 (cloud, vfs)
│   ├── cloud/         # 클라우드 관련 명령어
│   └── vfs/           # VFS 조작 명령어
├── cloud/             # 클라우드 스토리지 연동 (Google Drive)
├── cmd/               # 메인 진입점
│   ├── cli/           # CLI 애플리케이션 (main.go)
│   └── server/        # 웹 서버 애플리케이션 (main.go)
├── config/            # 설정 로직 (Viper, TOML)
├── drivers/           # VFS 스토리지 드라이버 인터페이스 및 구현체
│   ├── badger/        # BadgerDB (Key-Value) 드라이버
│   ├── localstorage/  # LocalStorage (Native FS) 드라이버
│   └── driver.go      # 드라이버 팩토리 및 공통 인터페이스
├── server/            # 웹 서버 및 API 라우팅 로직
│   ├── handlers/      # HTTP 핸들러 (VFS, SSE)
│   ├── middlewares/   # 미들웨어 (Logger, CORS 등)
│   ├── routes/        # 라우터 설정 (Web, API)
│   ├── services/      # 비즈니스 로직 (VFS Service, SSE Broker)
│   └── types/         # API 요청/응답 타입 정의
├── vfs.go             # VFS 인터페이스 및 코어 타입 (Meta, File, TreeNode)
├── webui/             # Svelte 5 + Vite 기반 웹 프론트엔드
├── go.mod             # Go 모듈 의존성 관리
├── Dockerfile         # Docker 이미지 빌드 설정
└── Makefile           # 빌드 및 유틸리티 스크립트 실행
```

## 3. 기능 리스트 및 요약

- **가상 파일 시스템 (VFS)**
  - **Core Interface**: `vfs.VFS` 인터페이스를 통한 표준화된 파일 작업.
  - **Pluggable Drivers**: `config.toml` 설정을 통해 스토리지 백엔드 교체 가능 (`badger`, `localstorage`).
  - **Meta Handling**: 파일 메타데이터(크기, 수정 시간, MIME 타입 등) 자동 관리.
- **웹 서버 (API)**
  - **Framework**: `gofiber/fiber` v3 기반의 고성능 웹 서버.
  - **API Endpoints**:
    - `POST /vfs`: 파일/디렉토리 생성
    - `GET /vfs`: 파일 목록 조회 (List/Tree 뷰 지원)
    - `GET /vfs/:id`: 파일 다운로드 및 스트리밍 (Range Request 지원)
    - `PUT /vfs/:id`: 파일 내용 수정
    - `PATCH /vfs/:id`: 파일/디렉토리 이동(Rename)
    - `DELETE /vfs/:id`: 파일/디렉토리 삭제
    - `POST /vfs/:id/copy`: 파일 복사
    - `PATCH /vfs/:id/comments`: 파일 코멘트 수정
    - **Maintenance**: `backup`, `restore`, `rotate` (암호화 키)
  - **SSE (Real-time)**:
    - `GET /sse/subscribe`: 실시간 이벤트 구독
    - `POST /sse/publish`: 이벤트 발행 (내부/외부 트리거)
    - **Async Operations**: 대용량 작업(Copy, Write 등)의 비동기 처리 및 진행 상황 알림.
- **웹 UI**
  - **Stack**: Svelte 5, Vite, TailwindCSS.
  - **Integration**: `server`가 정적 파일을 서빙하며 API와 연동.
- **CLI (Command Line Interface)**
  - **Framework**: `cobra`, `viper` 기반.
  - **Commands**:
    - `govfs [command]`: VFS 조작 및 관리 명령어 (Root Level)
      - `backup`, `restore`: 데이터 백업 및 복원
      - `rotate`: 암호화 키 교체
      - `ls`, `tree`, `stat`: 파일 조회
      - `cp`, `mkdir`, `rm`: 파일/디렉토리 조작
    - `govfs cloud [command]`: 클라우드 스토리지 관리
      - `list`: 파일 목록 조회
      - `upload`, `download`: 파일 전송

## 4. 아키텍처 상세 (Architecture Details)

### 4.1 클라이언트-서버 모델 (Client-Server Model)

**govfs**는 클라이언트-서버 아키텍처를 채택했습니다. 사용자는 CLI(Client)를 통해 명령을 내리고, 실제 파일 시스템 조작은 백그라운드에서 실행 중인 서버(Daemon)가 수행합니다.

- **Daemon (Server)**:
  - **역할**: 실제 스토리지(BadgerDB, LocalStorage)에 접근하여 I/O를 수행하고 상태를 관리합니다.
  - **통신**: HTTP REST API 및 SSE(Server-Sent Events)를 통해 클라이언트와 통신합니다.
- **CLI (Client)**:
  - **역할**: 사용자 명령을 파싱하여 서버 API를 호출하고 결과를 출력합니다.
  - **특징**: 무상태(Stateless)이며, 로컬 설정 파일이나 환경 변수를 통해 서버 연결 정보를 참조합니다.

### 4.2 드라이버 추상화 (Driver Abstraction)

`vfs.VFS` 인터페이스는 파일 시스템의 모든 동작을 추상화합니다. `drivers.New(config)` 팩토리 함수를 통해 설정에 맞는 구현체를 주입받습니다.

### 4.3 스토리지 엔진 (Storage Engines)

| 특징 | BadgerDB Driver | LocalStorage Driver |
| :--- | :--- | :--- |
| **Path** | `drivers/badger` | `drivers/localstorage` |
| **Backend** | BadgerDB (LSM Tree Key-Value Store) | Native OS Filesystem (`os`, `io` 패키지) |
| **Data Model** | Key: UUID / Value: Metadata + Content | File System Path |
| **Pros** | **Single File**: 운반 용이성.<br>**Encryption**: 데이터 암호화 내장.<br>**Transaction**:  ACID 보장. | **Performance**: OS 커널 캐시 활용.<br>**Accessibility**: 파일 직접 열람 가능.<br>**Debug**: 디버깅 용이. |

### 4.4 서버 아키텍처 (Server Architecture)

- **Handlers**: HTTP 요청을 처리하고 적절한 Service 메서드를 호출합니다. Fiber Context를 통해 요청 파라미터를 파싱합니다.
- **Services**: 비즈니스 로직을 수행합니다. `VfsService`는 `vfs.VFS` 인터페이스를 사용하여 실제 파일 작업을 수행합니다. `SSEBroker`는 클라이언트에게 실시간 이벤트를 브로드캐스트합니다.
- **Async Execution**: `Write`, `Copy`, `Move` 등 시간이 걸릴 수 있는 작업은 `SSEBroker.AsyncExcute`를 통해 별도 고루틴에서 실행되며, 완료 시 클라이언트에게 SSE 이벤트를 발송합니다.
