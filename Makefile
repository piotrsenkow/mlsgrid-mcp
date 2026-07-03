BINARY  := mlsgrid-mcp
VERSION ?= dev
LDFLAGS := -X github.com/piotrsenkow/mlsgrid-mcp/internal/cli.version=$(VERSION)

.PHONY: build run test test-integration lint fmt tidy docker-build clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

test:
	go test -race ./...

# Integration tests build a fixture database and are tagged //go:build integration.
# They need Docker (testcontainers). Land with the Postgres adapter tools (B-M2).
test-integration:
	go test -race -tags integration ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
	go vet ./...

tidy:
	go mod tidy

docker-build:
	docker build -t $(BINARY):$(VERSION) .

clean:
	rm -rf bin/
