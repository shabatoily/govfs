PRJ_NAME=govfs
GITHUB_USER=meteormin
AUTHOR="Meteormin \(miniyu97@gmail.com\)"
PRJ_BASE=$(shell pwd)
PRJ_DESC=$(PRJ_NAME) Deployment and Development Makefile.\n Author: $(AUTHOR)

SUPPORTED_OS=linux darwin
SUPPORTED_ARCH=amd64 arm64

DATE_UTC=$(shell date +"%Y-%m-%dT%H:%M:%S%z")

.DEFAULT: help
.SILENT:;

##help: helps (default)
.PHONY: help
help: Makefile
	echo ""
	echo " $(PRJ_DESC)"
	echo ""
	echo " Usage:"
	echo ""
	echo "	make {command}"
	echo ""
	echo " Commands:"
	echo ""
	sed -n 's/^##/	/p' $< | column -t -s ':' |  sed -e 's/^/ /'
	echo ""

# OS와 ARCH가 정의되어 있지 않으면 기본값을 설정합니다.
# uname -s는 OS 이름(예: Linux, Darwin 등)을 반환하고, tr를 통해 소문자로 변환합니다.
OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
# 아키텍처 정보를 반환합니다. (예: amd64, arm64 등)
ARCH := $(shell ./scripts/detect-arch)

##install: install development packages
.PHONY: install
install:
	@echo "[install] installing development packages"
	@echo "[install] go mod download"
	go mod download
	@echo "[install] go mod tidy"
	go mod tidy
	@echo "[install] yarn install"
	yarn --cwd webui install
	@echo "[install] complete install"

##build-webui
.PHONY: build-webui
build-webui:
	@echo "[build-webui] building webui"
	@echo "[build-webui] yarn build"
	yarn --cwd webui build
	@echo "[build-webui] complete build-webui"

##build os={os [linux, darwin]} arch={arch [amd64, arm64]} tag={tag [v1.0.0]}: build application
.PHONY: build
build: os ?= $(OS)
build: arch ?= $(ARCH)
build: tag ?= "0.0.1"
build: swag
build: build-webui
build:
	@echo "[build] building for $(os)/$(arch) at $(DATE_UTC)"
	@echo "[build] tag: $(tag)"
	@echo "[build] target: bin/$(PRJ_NAME)-$(os)-$(arch)"
	GOOS=$(os) GOARCH=$(arch) go build -trimpath -ldflags="-X 'main.version=$(tag)' -X 'main.buildTime=$(DATE_UTC)'" -o bin/$(PRJ_NAME)-$(os)-$(arch) cmd/server/main.go 
	@echo "[build] target: bin/$(PRJ_NAME)-cli-$(os)-$(arch)"
	GOOS=$(os) GOARCH=$(arch) go build -trimpath -ldflags="-X 'main.version=$(tag)' -X 'main.buildTime=$(DATE_UTC)'" -o bin/$(PRJ_NAME)-cli-$(os)-$(arch) cmd/cli/main.go 
	@echo "[build] Complete build"

##build-docker tag={tag [v1.0.0]}: build docker image
.PHONY: build-docker
build-docker: tag ?= "latest"
build-docker: swag
build-docker:
	@echo "[build-docker] building docker image at $(DATE_UTC)"
	@echo "[build-docker] tag: $(tag)"
	@echo "[build-docker] image: ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag)"
	docker build -t ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag) --build-arg "VERSION=$(tag)" --build-arg "BUILD_TIME=$(DATE_UTC)" .
	@echo "[build-docker] complete build-docker"

##release tag={tag [v1.0.0]}: release application
.PHONY: release
release: tag ?= "0.0.1"
release:
	@echo "[release] releasing at $(DATE_UTC)"
	@echo "[release] tag: $(tag)"
	@echo "[release] target: release/$(tag)"
	mkdir -p release/$(tag)
	$(foreach os, $(SUPPORTED_OS), \
		$(foreach arch, $(SUPPORTED_ARCH), \
			$(MAKE) build os=$(os) arch=$(arch)))
	cp bin/$(PRJ_NAME)-$(os)-$(arch) release/$(tag)
	cp config.yml release/$(tag)/config.yml

##release-docker tag={tag [v1.0.0]}: release application
.PHONY: release-docker
release-docker: tag ?= "latest"
release-docker:
	@echo "[release-docker] $(tag)"
	$(MAKE) build-docker tag=$(tag)
	docker tag ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag) ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):latest
	docker push ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):$(tag)
	docker push ghcr.io/$(GITHUB_USER)/$(PRJ_NAME):latest

##clean: clean application
.PHONY: clean
clean:
	@echo "[clean] Cleaning build directory"
	rm -rf bin/*
	rm -rf webui/dist/*

##clean-docker: clean docker
.PHONY: clean-docker
clean-docker:
	rm -rf .docker/*
	./scripts/docker-clean

##run args={args}: run application
.PHONY: run
run: args ?= ""
run: build-webui
run:
	@echo "[run] running application"
	@echo "args: $(args)"
	go run main.go $(args)

##test report={[0=inactive, 1=active]}: test
.PHONY: test
test:
ifeq ($(report), 1)
	@echo "[test] go test with report"
	mkdir -p reports
	go test -v ./... > reports/test.out
else
	@echo "[test] go test"
	go test ./...
endif

##benchmark: 📈 Benchmark code performance
.PHONY: benchmark
benchmark:
	@echo "[benchmark] starting benchmark $(PRJ_NAME)"
	go test ./... -benchmem -bench=. -run=^Benchmark$

##coverage: ☂️  Generate coverage report
.PHONY: coverage
coverage:
	go test -coverprofile=/tmp/coverage.out
	go tool cover -html=/tmp/coverage.out

##swag: generate api docs
.PHONY: swag
swag:
	swag init -g cmd/server/main.go --parseDependency --parseInternal --v3.1