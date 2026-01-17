IMAGE := elevenam/voice-backend

VERSION ?= $(shell v=$$(git tag -l "v*.*.*" --sort=-v:refname --points-at HEAD 2>/dev/null | head -1); echo "$${v:-dev}")
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

PLATFORMS := linux/amd64,linux/arm64

TAGS := -t $(IMAGE):$(VERSION) -t $(IMAGE):$(GIT_SHA) -t $(IMAGE):latest

.PHONY: all build push clean swagger test test-verbose coverage lint install \
        release-patch release-minor release-major \
        release-patch-push release-minor-push release-major-push help

.DEFAULT_GOAL := help

all: build

build:
	docker build $(TAGS) -f Dockerfile .

push:
	docker buildx build --platform $(PLATFORMS) $(TAGS) --no-cache --push -f Dockerfile .

swagger:
	swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal

clean:
	docker rmi $(IMAGE):$(VERSION) $(IMAGE):$(GIT_SHA) $(IMAGE):latest 2>/dev/null || true
	rm -f coverage.out coverage.html

test:
	go test ./...

test-verbose:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out $(or $(PKG),./...)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html (PKG=$(or $(PKG),./...))"

lint:
	golangci-lint run ./...

install:
	go mod download
	go mod tidy

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
	@echo "    make build          - Build voice-backend image"
	@echo "    make push           - Build and push voice-backend"
	@echo ""
	@echo "  Testing:"
	@echo "    make test           - Run all unit tests"
	@echo "    make test-verbose   - Run tests with verbose output"
	@echo "    make coverage       - Generate coverage (PKG=./internal/... for subset)"
	@echo "    make lint           - Run golangci-lint"
	@echo ""
	@echo "  Release (git tag only):"
	@echo "    make release-patch  - Create patch release (v0.0.X)"
	@echo "    make release-minor  - Create minor release (v0.X.0)"
	@echo "    make release-major  - Create major release (vX.0.0)"
	@echo ""
	@echo "  Release + Docker push:"
	@echo "    make release-patch-push  - Patch + push images"
	@echo "    make release-minor-push  - Minor + push images"
	@echo "    make release-major-push  - Major + push images"
	@echo ""
	@echo "  Other:"
	@echo "    make swagger        - Regenerate swagger docs"
	@echo "    make install        - Install/update dependencies"
	@echo "    make clean          - Remove images and coverage files"
	@echo ""
	@echo "  Variables:"
	@echo "    VERSION=$(VERSION)"
	@echo "    GIT_SHA=$(GIT_SHA)"
	@echo "    PLATFORMS=$(PLATFORMS)"
