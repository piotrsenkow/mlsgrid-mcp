BINARY  := mlsgrid-mcp
VERSION ?= dev
LDFLAGS := -X github.com/piotrsenkow/mlsgrid-mcp/internal/cli.version=$(VERSION)

.PHONY: build run test test-integration verify-pin lint fmt tidy docker-build clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

test:
	go test -race ./...

# Integration tests build a fixture database from the pinned mlsgrid-sync
# migration (the cross-repo contract test) and are tagged //go:build integration.
# They need Docker (testcontainers).
test-integration:
	go test -race -tags integration ./...

# Verify the vendored mlsgrid-sync migration still matches its upstream pin —
# keeps the cross-repo contract test honest. Runs in CI as the contract-drift job.
verify-pin:
	./scripts/verify-contract-pin.sh

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
