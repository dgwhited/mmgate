BINARY := mmgate
GO := go

.PHONY: build test lint security clean docker-build run

build:
	$(GO) build -ldflags="-s -w" -o $(BINARY) .

test:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...
	golangci-lint run ./...

security:
	gosec -exclude=G706 ./...
	govulncheck ./...

clean:
	rm -f $(BINARY)

docker-build:
	docker build -t $(BINARY):latest .

run: build
	./$(BINARY) --config config.yaml
