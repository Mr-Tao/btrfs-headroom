# SPDX-License-Identifier: Apache-2.0 OR MIT

VERSION ?= 0.1.0-dev
BUILD_DIR ?= build
DIST_DIR ?= dist

BINARY := $(BUILD_DIR)/bin/btrfs-headroom
MANPAGE := $(BUILD_DIR)/man/btrfs-headroom.1
BASH_COMPLETION := $(BUILD_DIR)/completions/bash/btrfs-headroom
ZSH_COMPLETION := $(BUILD_DIR)/completions/zsh/_btrfs-headroom
FISH_COMPLETION := $(BUILD_DIR)/completions/fish/btrfs-headroom.fish

.PHONY: all arm64-check build check check-all clean completions \
	completions-check docs docs-check fmt man man-check release shellcheck \
	systemd-check test test-race vet

all: build

build:
	mkdir -p "$(dir $(BINARY))"
	go build -trimpath -ldflags "-X main.version=$(VERSION)" \
		-o "$(BINARY)" ./cmd/btrfs-headroom

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test ./...

test-race:
	go test -race ./...

arm64-check:
	mkdir -p "$(BUILD_DIR)/arm64"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
		-o "$(BUILD_DIR)/arm64/btrfs-headroom" ./cmd/btrfs-headroom

man: $(MANPAGE)

$(MANPAGE): docs/man/btrfs-headroom.1.scd
	mkdir -p "$(dir $(MANPAGE))"
	scdoc < "$<" > "$@"

man-check:
	@set -eu; \
	tmp="$$(mktemp)"; \
	trap 'rm -f "$$tmp"' EXIT HUP INT TERM; \
	scdoc < docs/man/btrfs-headroom.1.scd > "$$tmp"; \
	groff -man -z -ww "$$tmp"

docs: man

docs-check: man-check

completions: build
	mkdir -p "$(dir $(BASH_COMPLETION))" \
		"$(dir $(ZSH_COMPLETION))" "$(dir $(FISH_COMPLETION))"
	"$(BINARY)" completion bash > "$(BASH_COMPLETION)"
	"$(BINARY)" completion zsh > "$(ZSH_COMPLETION)"
	"$(BINARY)" completion fish > "$(FISH_COMPLETION)"

completions-check: completions
	bash -n "$(BASH_COMPLETION)"
	shellcheck -s bash "$(BASH_COMPLETION)"
	zsh -n "$(ZSH_COMPLETION)"
	fish -n "$(FISH_COMPLETION)"

shellcheck:
	shellcheck contrib/systemd/user/btrfs-headroom-notify \
		scripts/package-release.sh

systemd-check:
	systemd-analyze verify \
		contrib/systemd/system/btrfs-headroom.service \
		contrib/systemd/system/btrfs-headroom.timer
	systemd-analyze --user verify \
		contrib/systemd/user/btrfs-headroom-notify.service \
		contrib/systemd/user/btrfs-headroom-notify.path

check-all: check test-race man-check completions-check shellcheck systemd-check

release:
	@test "$(VERSION)" != "0.1.0-dev" || { \
		echo "VERSION must be a release version such as 0.1.0" >&2; \
		exit 2; \
	}
	scripts/package-release.sh "$(VERSION)" "$(DIST_DIR)"

clean:
	rm -rf "$(BUILD_DIR)" "$(DIST_DIR)"
