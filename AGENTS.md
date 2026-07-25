<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Agent Guidance

## Safety Invariant

- The observed Btrfs filesystem is read-only from this project's perspective.
- Do not add balance, resize, repair, scrub, defragment, delete, or arbitrary
  command execution features.
- An output path or the tool's own state directory is the only intentional
  write target.

## Validation

- Run `gofmt` on changed Go files.
- Run `go vet ./...` and `go test ./...`.
- Keep observation schema changes backward-compatible within a schema version.
- Unknown or incomplete allocator state must never be reported as healthy.
