VERSION ?= 0.1.0
MODULE  := github.com/dsmmcken/dh-cli/src
LDFLAGS := -ldflags="-s -w -X $(MODULE)/internal/cmd.Version=$(VERSION)"
BINARY  := dhg
PKG_NAME := dhg-cli

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

# go-to-wheel platform strings (uses dashes, not slashes)
WHEEL_PLATFORMS_ALL := linux-amd64,linux-arm64,darwin-amd64,darwin-arm64,windows-amd64,windows-arm64

# Detect current platform for single-platform wheel builds
UNAME_S := $(shell uname -s | tr A-Z a-z)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),linux)
  CURRENT_OS := linux
else ifeq ($(UNAME_S),darwin)
  CURRENT_OS := darwin
else
  CURRENT_OS := windows
endif
ifeq ($(UNAME_M),x86_64)
  CURRENT_ARCH := amd64
else ifeq ($(UNAME_M),aarch64)
  CURRENT_ARCH := arm64
else ifeq ($(UNAME_M),arm64)
  CURRENT_ARCH := arm64
else
  CURRENT_ARCH := amd64
endif
WHEEL_PLATFORM := $(CURRENT_OS)-$(CURRENT_ARCH)

GO_TO_WHEEL := uvx go-to-wheel
GO_TO_WHEEL_ARGS := . \
	--name $(PKG_NAME) \
	--version $(VERSION) \
	--entry-point $(BINARY) \
	--set-version-var $(MODULE)/internal/cmd.Version \
	--description "Deephaven CLI" \
	--readme ../README.md \
	--output-dir dist

.PHONY: build build-all test clean package package-all install-local uninstall

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/dhg

build-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output="dist/$(BINARY)-$${os}-$${arch}"; \
		if [ "$$os" = "windows" ]; then output="$${output}.exe"; fi; \
		echo "Building $$output ..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o $$output ./cmd/dhg; \
	done

test:
	cd ../unit_tests && go test ./...
	cd ../behaviour_tests && go test ./...

clean:
	rm -f $(BINARY)
	rm -rf dist

## Package the current platform as a Python wheel using go-to-wheel
package:
	$(GO_TO_WHEEL) $(GO_TO_WHEEL_ARGS) --platforms $(WHEEL_PLATFORM)

## Package all platforms as Python wheels using go-to-wheel
package-all:
	$(GO_TO_WHEEL) $(GO_TO_WHEEL_ARGS) --platforms $(WHEEL_PLATFORMS_ALL)

## Build wheel for current platform and install via uv tool
install-local: package
	uv tool install --force dist/dhg_cli-$(VERSION)-*.whl

## Uninstall dhg-cli from uv tools
uninstall:
	uv tool uninstall $(PKG_NAME)
