# AGENTS.md

이 문서는 프로젝트에서 AI 에이전트가 따라야 하는 공통 규칙과 워크플로우를 정리합니다.

## 공통 규칙

### Codebase

Go 언어에서 권장하는 표준을 준수합니다.

- Linter: `golangci-lint`
- LSP: `gopls`
- Formatter: `gofumpt`

### Comments

- 코드 주석은 한글로 작성합니다. 단, 에러 메시지와 로그/콘솔 출력은 제외합니다.
- 고유명사이거나 한글로 번역하면 의미 해석이 어색해지는 경우에는 판단에 따라 영어로 작성합니다.
- 코멘트는 간결하게 핵심만 작성합니다.
- 성능 및 구현 공수 때문에 코드가 복잡하거나 가독성이 떨어지는 경우에는 상세하게 코멘트를 작성합니다.

## 빌드 가이드

- Backend server: `Makefile` 및 `go build`
- Frontend webui: `yarn`, `vite`
- 빌드 관련 커맨드는 [Makefile](Makefile)을 참조합니다.

## 워크플로우

### Branch

작업을 시작하기 전에 현재 브랜치와 변경 사항을 확인합니다.

```bash
git status --short --branch
```

현재 브랜치가 `main`이거나 브랜치의 목적과 작업 내용이 다르면 새 브랜치를 생성합니다.

```bash
git switch -c <type>/<description> main
```

브랜치 이름은 소문자 kebab-case로 작성합니다.

- `feature/<description>`: 기능 추가
- `fix/<description>`: 버그 수정
- `refactor/<description>`: 동작 변경 없는 구조 개선
- `docs/<description>`: 문서 변경
- `test/<description>`: 테스트 변경
- `chore/<description>`: 빌드, 설정 및 유지보수

예: `feature/mcp-tools`, `fix/sse-parser`, `docs/pr-workflow`

### Commit

현재 변경 사항 전체를 빠르게 검증하고, Conventional Commits 형식의 커밋 메시지로 커밋하는 워크플로우입니다.

1. 패키지 취약점 점검

   ```bash
   make audit
   ```

2. 빌드 가능 여부 확인

   ```bash
   go build ./...
   ```

3. 변경 사항 스테이징

   ```bash
   git add .
   ```

4. 스테이징된 변경 사항 분석

   ```bash
   git diff --cached
   ```

5. Conventional Commits 형식으로 커밋 메시지를 생성하고 커밋

   ```bash
   git commit -m "<ai_generated_message>"
   ```

6. 필요 시 변경 사항 푸시

   ```bash
   git push
   ```

### Pull Request Body

현재 브랜치가 `main`이 아닐 때만 `.github/pull_request_template.md` 양식으로 PR 본문을 작성합니다.

1. 현재 브랜치를 확인합니다.

   ```bash
   git branch --show-current
   ```

2. `main`과 비교해 커밋 및 변경 사항을 확인합니다.

   ```bash
   git log --oneline main..HEAD
   git diff --stat main...HEAD
   git diff main...HEAD
   ```

3. 변경 이유와 핵심 변경 사항만 간결하게 작성하고, 실제로 수행한 검증만 표시합니다.

4. PR 본문은 바로 복사할 수 있도록 하나의 Markdown 코드 블록으로 출력합니다.

5. 현재 브랜치가 `main`이면 PR 본문을 작성하지 않습니다.

### Documentation Analysis and DESIGN.md Synthesis

이 워크플로우의 목적은 프로젝트의 기존 문서, 주석, 실제 코드를 단계적으로 분석하여 실제 구현과 일치하는 최신 `DESIGN.md` 파일을 생성하거나 수정하는 것입니다.

#### 분석 원칙

- Source of Truth: 문서와 코드가 상충할 경우 실제 동작하는 코드를 최우선 순위로 둡니다.
- Incremental Update: 기존 `DESIGN.md`가 존재할 경우 무조건 덮어쓰기보다 변경 사항을 추적하여 논리적으로 수정합니다.
- Evidence-based: 모든 설명은 프로젝트 내의 물리적 파일 경로와 연결되어야 합니다.

#### 실행 단계

1. 문서 우선 탐색

   - 대상: 프로젝트 루트 및 `docs/` 디렉터리 내의 모든 `.md` 파일
   - 목표: 프로젝트의 설계 의도, 비즈니스 로직의 배경, 아키텍처 가이드를 파악합니다.
   - 결과: 가상의 설계 지도를 생성합니다.

2. 코드 주석 분석

   - 대상: 소스 코드 전체
   - 목표: 클래스, 메서드 상단에 기술된 Javadoc, Docstring 등을 추출합니다.
   - 비교: 문서 우선 탐색에서 파악한 설계 의도가 실제 인터페이스 명세와 일치하는지 대조합니다.

3. 실구현 코드 분석

   - 대상: 실제 비즈니스 로직 구현체 (`.java`, `.py`, `.ts`, `.go` 등)
   - 목표: 실제 호출 흐름, 데이터 저장 방식, 예외 처리 로직을 분석합니다.
   - 대조 검증:
     - 문서에는 존재하지만 코드에는 없는 기능을 식별합니다.
     - 코드에는 구현되었으나 문서/주석에 누락된 로직을 식별합니다.
     - 기존 분석 결과와 실제 코드 간의 논리적 모순을 기록합니다.

4. `DESIGN.md` 생성 및 수정

   - 대상: 프로젝트 루트의 `DESIGN.md`
   - 구성:
     - Architecture Overview: 프로젝트의 전체적인 구조
     - Component/Module Details: 분석된 실제 모듈별 역할 정의
     - Implementation vs Design: 문서와 실제 코드 간의 주요 차이점
     - Updated Date: 분석 완료 시간 및 기준 커밋 정보

#### 에이전트 준수 사항

- 분석 중 발견된 모든 불일치(Discrepancy)는 무시하지 말고 `DESIGN.md`에 `Notes` 또는 `Known Limitations` 섹션으로 기록합니다.
- 가급적 표준 기술 용어(Design Patterns, Architecture Styles)를 사용하여 전문성을 유지합니다.
- 출력물(`DESIGN.md`)의 언어 설정은 사용자의 별도 지시가 없다면 프로젝트의 주 사용 언어를 따릅니다.
