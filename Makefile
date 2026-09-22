BINARY  := urga
PKG     := github.com/ingvarch/urga
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)

GOOS  ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# The archive of one platform: a zip for Windows, a tarball everywhere else.
ARCHIVE := urga_$(VERSION)_$(GOOS)_$(GOARCH)
EXT     := $(if $(filter windows,$(GOOS)),.exe,)

.PHONY: check fmt vet lint test build dist run clean

check: fmt vet lint test build

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

test:
	go test ./...

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

# dist builds one platform and packs it the way a release is downloaded.
dist:
	rm -rf dist/$(ARCHIVE)
	mkdir -p dist/$(ARCHIVE)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o dist/$(ARCHIVE)/$(BINARY)$(EXT) ./cmd/$(BINARY)
	cp README.md LICENSE dist/$(ARCHIVE)/
ifeq ($(GOOS),windows)
	cd dist && zip -qr $(ARCHIVE).zip $(ARCHIVE)
else
	cd dist && tar -czf $(ARCHIVE).tar.gz $(ARCHIVE)
endif
	rm -rf dist/$(ARCHIVE)

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin
