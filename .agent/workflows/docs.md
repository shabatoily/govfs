---
description: AI Agent Workflow: Documentation Analysis & DESIGN.md Synthesis
---

워크플로우의 목적은 프로젝트의 기존 문서, 주석, 그리고 실제 코드를 단계적으로 분석하여 실제 구현과 일치하는 최신화된 `DESIGN.md` 파일을 생성하거나 수정하는 것입니다.

## 2. 분석 원칙
- **Source of Truth (진실의 근원)**: 문서와 코드가 상충할 경우, 실제 동작하는 **코드**를 최우선 순위로 둡니다.
- **Incremental Update**: 기존 `DESIGN.md`가 존재할 경우, 무조건적인 덮어쓰기보다 변경 사항을 추적하여 논리적으로 수정합니다.
- **Evidence-based**: 모든 설명은 프로젝트 내의 물리적 파일 경로와 연결되어야 합니다.

## 3. 실행 단계 (Execution Steps)

### Step 1: 문서 우선 탐색 (Document Analysis)
- **대상**: 프로젝트 루트 및 `docs/` 디렉토리 내의 모든 `.md` 파일.
- **목표**: 프로젝트의 설계 의도, 비즈니스 로직의 배경, 아키텍처 가이드를 파악합니다.
- **결과**: 가상의 설계 지도를 생성합니다.

### Step 2: 코드 주석 분석 (Spec Extraction)
- **대상**: 소스 코드 전체 (`src/` 등).
- **목표**: 클래스, 메서드 상단에 기술된 Javadoc, Docstring 등을 추출합니다.
- **비교**: Step 1에서 파악한 설계 의도가 실제 인터페이스 명세(주석)와 일치하는지 대조합니다.

### Step 3: 실구현 코드 분석 (Code Deep-Dive)
- **대상**: 실제 비즈니스 로직 구현체 (`.java`, `.py`, `.ts`, `.go` 등).
- **목표**: 실제 호출 흐름, 데이터 저장 방식, 예외 처리 로직을 분석합니다.
- **대조 검증 (Critical)**:
    - 문서에는 존재하지만 코드에는 없는 기능 식별.
    - 코드에는 구현되었으나 문서/주석에 누락된 로직 식별.
    - 기존 분석 결과와 실제 코드 간의 논리적 모순 기록.

### Step 4: DESIGN.md 생성 및 수정 (Synthesis)
- **대상**: 프로젝트 루트의 `DESIGN.md`.
- **구성**:
    1. **Architecture Overview**: 프로젝트의 전체적인 구조.
    2. **Component/Module Details**: 분석된 실제 모듈별 역할 정의.
    3. **Implementation vs Design**: 문서와 실제 코드 간의 주요 차이점(있는 경우).
    4. **Updated Date**: 분석 완료 시간 및 기준 커밋 정보(가능할 경우).

## 4. 에이전트 준수 사항
- 분석 중 발견된 모든 "불일치(Discrepancy)"는 무시하지 말고 `DESIGN.md`에 '비고(Notes)' 또는 'Known Limitations' 섹션으로 기록할 것.
- 가급적 표준 기술 용어(Design Patterns, Architecture Styles)를 사용하여 전문성을 유지할 것.
- 출력물(`DESIGN.md`)의 언어 설정은 사용자의 별도 지시가 없다면 프로젝트의 주 사용 언어를 따를 것.