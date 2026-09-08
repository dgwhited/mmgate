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

docker-build:
	docker build -t $(BINARY):latest .

run: build
	./$(BINARY) --config config.yaml
