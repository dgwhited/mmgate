BINARY := mmgate
GO := go

.PHONY: build test lint clean docker-build run

build:
	$(GO) build -ldflags="-s -w" -o $(BINARY) .

test:
	$(GO) test ./...

lint:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)

docker-build:
	docker build -t $(BINARY):latest .

run: build
	./$(BINARY) --config config.yaml
