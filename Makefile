BINARY := mmgate
GO := go

# Pinned tool versions — keep in sync with .github/workflows/ci.yaml.
GOLANGCI_LINT_VERSION := v2.13.2
GOSEC_VERSION := v2.29.0

# The `go` directive in go.mod is the single source of truth for the toolchain.
# govulncheck must be *built* with a toolchain at least as new as the code it
# analyses, otherwise it fails with "package requires newer Go version".
GO_VERSION := $(shell sed -n 's/^go \([0-9][0-9.]*\)$$/\1/p' go.mod)
GOBIN := $(shell $(GO) env GOPATH)/bin

.PHONY: build test tidy lint security check clean docker-build run

build:
	$(GO) build -ldflags="-s -w" -o $(BINARY) .

test:
	$(GO) test -race -coverprofile=coverage.out ./...

tidy:
	$(GO) mod tidy

lint:
	$(GO) vet ./...
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

security:
	GOTOOLCHAIN=go$(GO_VERSION) $(GO) install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	$(GOBIN)/gosec ./...
	GOTOOLCHAIN=go$(GO_VERSION) $(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GOBIN)/govulncheck ./...

# Everything CI runs, in one target.
check: lint test security

clean:
	rm -f $(BINARY) coverage.out

# Prefer docker, fall back to podman. Podman defaults to the OCI image format,
# which silently drops HEALTHCHECK ("not supported for OCI image format"), so
# ask it for the docker format explicitly. Buildx (used by goreleaser in CI)
# already produces docker-format images.
CONTAINER_ENGINE ?= $(shell command -v docker >/dev/null 2>&1 && echo docker || echo podman)
ifeq ($(CONTAINER_ENGINE),podman)
CONTAINER_BUILD_FLAGS := --format docker
endif

# The Dockerfile copies a pre-built binary, matching how goreleaser assembles
# the build context, so cross-compile for linux first.
docker-build:
	GOOS=linux GOARCH=$(shell $(GO) env GOARCH) CGO_ENABLED=0 \
		$(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) .
	$(CONTAINER_ENGINE) build $(CONTAINER_BUILD_FLAGS) -t $(BINARY):latest .

run: build
	./$(BINARY) --config config.yaml
