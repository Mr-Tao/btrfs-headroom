# SPDX-License-Identifier: Apache-2.0 OR MIT

.PHONY: build check fmt test vet

build:
	go build ./cmd/btrfs-headroom

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
