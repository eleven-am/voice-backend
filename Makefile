BACKEND_IMAGE := elevenam/voice-backend
SIDECAR_IMAGE := elevenam/voice-sidecar

VERSION ?= $(shell v=$$(git tag -l "v*.*.*" --sort=-v:refname --points-at HEAD 2>/dev/null | head -1); echo "$${v:-dev}")
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

BACKEND_PLATFORMS := linux/amd64,linux/arm64
SIDECAR_PLATFORMS := linux/amd64

BACKEND_TAGS := -t $(BACKEND_IMAGE):$(VERSION) -t $(BACKEND_IMAGE):$(GIT_SHA) -t $(BACKEND_IMAGE):latest
SIDECAR_TAGS := -t $(SIDECAR_IMAGE):$(VERSION) -t $(SIDECAR_IMAGE):$(GIT_SHA) -t $(SIDECAR_IMAGE):latest

.PHONY: all build push clean backend sidecar backend-push sidecar-push \
        swagger proto test test-verbose test-sidecar coverage lint install \
        run run-backend run-sidecar \
        release-patch release-minor release-major \
        release-patch-push release-minor-push release-major-push help

.DEFAULT_GOAL := help

all: build

build: backend sidecar

push: backend-push sidecar-push

backend:
	docker build $(BACKEND_TAGS) -f Dockerfile .

backend-push:
	docker buildx build --platform $(BACKEND_PLATFORMS) $(BACKEND_TAGS) --no-cache --push -f Dockerfile .

sidecar:
	docker build $(SIDECAR_TAGS) -f sidecar/Dockerfile ./sidecar

sidecar-push:
	docker buildx build --platform $(SIDECAR_PLATFORMS) $(SIDECAR_TAGS) --no-cache --push -f sidecar/Dockerfile ./sidecar

swagger:
	swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal

proto:
	@echo "Generating Go protobuf code..."
	protoc --go_out=. --go-grpc_out=. \
		--go_opt=module=github.com/eleven-am/voice-backend \
		--go-grpc_opt=module=github.com/eleven-am/voice-backend \
		proto/stt.proto proto/tts.proto
	@echo "Generating Python protobuf code..."
	python3 -m grpc_tools.protoc \
		-Iproto \
		--python_out=sidecar/sidecar \
		--grpc_python_out=sidecar/sidecar \
		--pyi_out=sidecar/sidecar \
		proto/stt.proto proto/tts.proto
	@echo "Fixing Python grpc imports for relative paths..."
	sed -i '' 's/^import stt_pb2 as/from . import stt_pb2 as/' sidecar/sidecar/stt_pb2_grpc.py
	sed -i '' 's/^import tts_pb2 as/from . import tts_pb2 as/' sidecar/sidecar/tts_pb2_grpc.py
	@echo "Proto generation complete."

clean:
	docker rmi $(BACKEND_IMAGE):$(VERSION) $(BACKEND_IMAGE):$(GIT_SHA) $(BACKEND_IMAGE):latest 2>/dev/null || true
	docker rmi $(SIDECAR_IMAGE):$(VERSION) $(SIDECAR_IMAGE):$(GIT_SHA) $(SIDECAR_IMAGE):latest 2>/dev/null || true
	rm -f coverage.out coverage.html
	rm -rf sidecar/.pytest_cache sidecar/.coverage

test:
	go test ./...

test-verbose:
	go test -v ./...

test-sidecar:
	cd sidecar && uv run pytest tests/ -v

test-all: test test-sidecar

coverage:
	go test -coverprofile=coverage.out $(or $(PKG),./...)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html (PKG=$(or $(PKG),./...))"

coverage-sidecar:
	cd sidecar && uv run pytest tests/ --cov=sidecar --cov-report=html

lint:
	golangci-lint run ./...

lint-sidecar:
	cd sidecar && uv run ruff check .

lint-all: lint lint-sidecar

install:
	go mod download
	go mod tidy

install-sidecar:
	cd sidecar && uv sync

install-all: install install-sidecar

run-backend:
	go run cmd/server/main.go

run-sidecar:
	cd sidecar && uv run sidecar serve --stt-port 50052 --tts-port 50053

define release
	@latest_tag=$$(git tag -l "v*.*.*" | sort -V | tail -1); \
	current=$${latest_tag:-v0.0.0}; \
	new=$$(echo $$current | awk -F. '$(1)'); \
	echo "$$current -> $$new"; \
	git tag $$new && git push origin $$new
endef

release-patch:
	$(call release,{$$3 = $$3 + 1; print $$1"."$$2"."$$3})

release-minor:
	$(call release,{$$2 = $$2 + 1; $$3 = 0; print $$1"."$$2"."$$3})

release-major:
	$(call release,{split($$1,a,"v"); printf "v%d.0.0"\, a[2]+1})

release-patch-push: release-patch
	$(MAKE) push

release-minor-push: release-minor
	$(MAKE) push

release-major-push: release-major
	$(MAKE) push

help:
	@echo "Usage:"
	@echo ""
	@echo "  Docker:"
	@echo "    make backend         - Build voice-backend image"
	@echo "    make backend-push    - Build and push voice-backend"
	@echo "    make sidecar         - Build voice-sidecar image"
	@echo "    make sidecar-push    - Build and push voice-sidecar"
	@echo "    make build           - Build both images"
	@echo "    make push            - Build and push both images"
	@echo ""
	@echo "  Development:"
	@echo "    make run-backend     - Run Go backend server"
	@echo "    make run-sidecar     - Run Python sidecar (STT/TTS)"
	@echo ""
	@echo "  Testing:"
	@echo "    make test            - Run Go unit tests"
	@echo "    make test-verbose    - Run Go tests with verbose output"
	@echo "    make test-sidecar    - Run Python sidecar tests"
	@echo "    make test-all        - Run all tests"
	@echo "    make coverage        - Generate Go coverage (PKG=./internal/... for subset)"
	@echo "    make coverage-sidecar - Generate Python coverage"
	@echo ""
	@echo "  Linting:"
	@echo "    make lint            - Run golangci-lint on Go code"
	@echo "    make lint-sidecar    - Run ruff on Python code"
	@echo "    make lint-all        - Run all linters"
	@echo ""
	@echo "  Dependencies:"
	@echo "    make install         - Install Go dependencies"
	@echo "    make install-sidecar - Install Python dependencies (uv sync)"
	@echo "    make install-all     - Install all dependencies"
	@echo ""
	@echo "  Code Generation:"
	@echo "    make proto           - Regenerate Go + Python protobuf code"
	@echo "    make swagger         - Regenerate swagger docs"
	@echo ""
	@echo "  Release (git tag only):"
	@echo "    make release-patch   - Create patch release (v0.0.X)"
	@echo "    make release-minor   - Create minor release (v0.X.0)"
	@echo "    make release-major   - Create major release (vX.0.0)"
	@echo ""
	@echo "  Release + Docker push:"
	@echo "    make release-patch-push  - Patch + push images"
	@echo "    make release-minor-push  - Minor + push images"
	@echo "    make release-major-push  - Major + push images"
	@echo ""
	@echo "  Cleanup:"
	@echo "    make clean           - Remove images and coverage files"
	@echo ""
	@echo "  Variables:"
	@echo "    VERSION=$(VERSION)"
	@echo "    GIT_SHA=$(GIT_SHA)"
	@echo "    BACKEND_PLATFORMS=$(BACKEND_PLATFORMS)"
	@echo "    SIDECAR_PLATFORMS=$(SIDECAR_PLATFORMS)"
