PRJ_NAME=govfs
GITHUB_USER=meteormin
AUTHOR="Meteormin \(miniyu97@gmail.com\)"
PRJ_BASE=$(shell pwd)
PRJ_DESC=$(PRJ_NAME) Deployment and Development Makefile.

SUPPORTED_OS=linux darwin
SUPPORTED_ARCH=amd64 arm64

DATE_UTC=$(shell date +"%Y-%m-%dT%H:%M:%S%z")

# OS와 ARCH가 정의되어 있지 않으면 기본값을 설정합니다.
# go env를 통해 현재 시스템의 OS와 ARCH를 가져옵니다.
OS=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)

SWAGGO_VERSION=v2.0.0-rc5

.DEFAULT: help
.SILENT:;

##help: helps (default)
.PHONY: help
help: Makefile
	@echo ""
	@echo " $(PRJ_DESC)"
	@echo ""
	@echo " Author: $(AUTHOR)"
	@echo ""
	@echo " OS: $(OS)"
	@echo " ARCH: $(ARCH)"
	@echo ""
	@echo " Usage:"
	@echo ""
	@echo "	make {command}"
	@echo ""
	@echo " Commands:"
	@echo ""
	@sed -n 's/^##/	/p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo ""

##audit: 🚀 Conduct quality checks
.PHONY: audit
audit:
	@echo "[audit] starting audit"
	@go mod verify
	@go vet ./...
	@GOTOOLCHAIN=$(GOVERSION) go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@echo "[audit] complete audit"

##benchmark: 📈 Benchmark code performance
.PHONY: benchmark
benchmark:
	@echo "[benchmark] starting benchmark $(PRJ_NAME)"
	@go test ./... -benchmem -bench=. -run=^Benchmark$
	@echo "[benchmark] complete benchmark"

##build os={os [linux, darwin]} arch={arch [amd64, arm64]} tag={tag [v1.0.0]}: build application
.PHONY: build
build: os ?= $(OS)
build: arch ?= $(ARCH)
build: tag ?= "0.0.1"
build: build-webui
build: swag
build:
	@echo "[build] building for $(os)/$(arch) at $(DATE_UTC)"
	@echo "[build] tag: $(tag)"
	@echo "[build] target: bin/$(PRJ_NAME)-$(os)-$(arch)"
	@GOOS=$(os) GOARCH=$(arch) go build -trimpath -ldflags="-X 'main.version=$(tag)' -X 'main.buildTime=$(DATE_UTC)'" -o bin/$(PRJ_NAME)-$(os)-$(arch) cmd/server/main.go 
	@echo "[build] target: bin/$(PRJ_NAME)-cli-$(os)-$(arch)"
	@GOOS=$(os) GOARCH=$(arch) go build -trimpath -ldflags="-X 'main.version=$(tag)' -X 'main.buildTime=$(DATE_UTC)'" -o bin/$(PRJ_NAME)-cli-$(os)-$(arch) cmd/cli/main.go 
	@echo "[build] Complete build"

##build-docker tag={tag [v1.0.0]}: build docker image
.PHONY: build-docker
build-docker: tag ?= "latest"
build-docker: swag
build-docker:
	@echo "[build-docker] building docker image at $(DATE_UTC)"
	@echo "[build-docker] tag: $(tag)"
	@echo "[build-docker] image: ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag)"
	@docker build -t ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag) --build-arg "VERSION=$(tag)" --build-arg "BUILD_TIME=$(DATE_UTC)" .
	@echo "[build-docker] complete build-docker"

##build-webui
.PHONY: build-webui
build-webui:
	@echo "[build-webui] building webui"
	@echo "[build-webui] yarn build"
	@yarn --cwd webui build
	@echo "[build-webui] complete build-webui"

##clean: clean project build and cache
.PHONY: clean
clean:
	@echo "[clean] Cleaning project build and cache"
	@echo "[clean] remove build output directory"
	@rm -rf bin/*
	@rm -rf webui/dist/*
	@echo "[clean] clean go cache"
	@go clean -cache
	@go clean -modcache
	@echo "[clean] clean yarn cache"
	@yarn --cwd webui cache clean
	@echo "[clean] clear node_modules"
	@rm -rf webui/node_modules
	@echo "[clean] complete clean"

##clean-docker: clean docker
.PHONY: clean-docker
clean-docker:
	@echo "[clean-docker] cleaning docker"
	@rm -rf .docker/*
	@./scripts/docker-clean
	@echo "[clean-docker] complete clean-docker"

##coverage: ☂️  Generate coverage report
.PHONY: coverage
coverage:
	@echo "[coverage] starting coverage"
	@go test ./... -coverprofile=/tmp/coverage.out
	@echo "[coverage] generating coverage report"
	@go tool cover -html=/tmp/coverage.out
	@echo "[coverage] complete coverage"

##install: install development packages
.PHONY: install
install:
	@echo "[install] installing development packages"
	@echo "[install] go mod download"
	@go mod download
	@echo "[install] go mod tidy"
	@go mod tidy -v
	@echo "[install] go install github.com/swaggo/swag/v2/cmd/swag@$(SWAGGO_VERSION)"
	@go install github.com/swaggo/swag/v2/cmd/swag@$(SWAGGO_VERSION)
	@echo "[install] yarn install"
	@yarn --cwd webui install
	@echo "[install] complete install"

##lint: 🚨 Run lint checks
.PHONY: lint
lint:
	@echo "[lint] starting lint"
	@GOTOOLCHAIN=$(GOVERSION) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 run ./...
	@echo "[lint] complete lint"

##release tag={tag [v1.0.0]}: release application
.PHONY: release
release: tag ?= "0.0.1"
release:
	@echo "[release] releasing at $(DATE_UTC)"
	@echo "[release] tag: $(tag)"
	@echo "[release] target: release/"
	@rm -rf release/*
	@mkdir -p release
	$(foreach os, $(SUPPORTED_OS), \
		$(foreach arch, $(SUPPORTED_ARCH), \
			$(MAKE) build os=$(os) arch=$(arch) && \
			cp bin/$(PRJ_NAME)-$(os)-$(arch) release/ && \
			cp bin/$(PRJ_NAME)-cli-$(os)-$(arch) release/ ; \
		) \
	)
	@cp config.toml release/config.toml

##release-docker tag={tag [v1.0.0]}: release application
.PHONY: release-docker
release-docker: tag ?= "latest"
release-docker:
	@echo "[release-docker] starting release-docker"
	$(MAKE) build-docker tag=$(tag)
	@echo "[release-docker] pushing docker image"
	docker tag ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag) ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):latest
	docker push ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag)
	docker push ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):latest
	@echo "[release-docker] complete release-docker"

##swag: generate api docs
.PHONY: swag
swag:
	@echo "[swag] generating api docs"
	go run github.com/swaggo/swag/v2/cmd/swag@$(SWAGGO_VERSION) init -g cmd/server/main.go --parseDependency --parseInternal --v3.1
	@echo "[swag] complete swag"

##test report={[0=inactive, 1=active]}: test
.PHONY: test
test: test-webui
test:
	@echo "[test] starting test"
ifeq ($(report), 1)
	@echo "[test] go test with report"
	mkdir -p reports
	go test -v ./... > reports/test.out
else
	@echo "[test] go test"
	go test ./...
endif
	@echo "[test] complete test"

##test-webui: test webui
.PHONY: test-webui
test-webui:
	@echo "[test-webui] starting test webui"
	cd webui && yarn test run
	@echo "[test-webui] complete test webui"
