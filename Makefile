BINARY  := urga
PKG     := github.com/ingvarch/urga
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)

.PHONY: check fmt vet lint licenses notices test build dist run screens clean

check: fmt vet lint licenses test build

fmt:
	gofmt -l -w .

vet:
	go vet ./...

# CI lints with the version named in .tool-versions. Another version here
# can pass what CI fails.
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed: CI lints with the version in .tool-versions"; \
		exit 1; \
	}
	golangci-lint run ./...

# licenses fails when a module urga links, on any platform the release
# builds for, has a license outside internal/licenses.Allowed. notices checks
# the same and writes THIRD_PARTY_NOTICES, which the release ships.
licenses:
	go run ./internal/licenses/cmd/licenses ./cmd/urga

notices:
	go run ./internal/licenses/cmd/licenses -notices THIRD_PARTY_NOTICES ./cmd/urga

test:
	go test ./...

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

# dist builds every platform and package the way a release does, and
# publishes nothing.
dist:
	goreleaser release --snapshot --clean

run: build
	./bin/$(BINARY)

# screens retakes the pictures of urga in the README.
screens:
	./scripts/screens.sh

clean:
	rm -rf bin THIRD_PARTY_NOTICES
