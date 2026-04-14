---
trigger: always_on
---

# Codebase

Go 언어에서 권장하는 표준을 준수

- Linter: golangci-lint
- LSP: gopls
- Formatter: gofumpt

## Comments
- 코드 주석은 한글로 작성합니다.(에러 메시지, 로그/콘솔 출력은 제외)
- 고유명사 혹은 한글로 번역 시 오히려 의미 해석이 어색해지는 경우는 판단하에 영어로 작성
- 코멘트는 간결하게 핵심만 작성합니다.
- 성능 및 구현 공수 때문에 복잡하거나 가독성이 떨어지는 경우는 상세하게 코멘트를 작성합니다.