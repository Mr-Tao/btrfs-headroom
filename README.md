<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# btrfs-headroom

`btrfs-headroom` is a read-only Linux health check for Btrfs allocator
headroom. It is designed to warn about the class of ENOSPC failure where
`df` still reports free space but Btrfs has too little raw, unallocated device
space to grow a pressured metadata allocation.

The project is in early development. The current CLI and report schema are
usable, but policy defaults and output details may change during the 0.x
release series.

## Why `df` is not enough

Btrfs first allocates large block groups ("chunks") from raw device space and
then stores data or metadata inside those chunks. `df` reports a logical
filesystem estimate. It does not directly answer whether Btrfs can allocate
the next metadata chunk on the underlying device or devices.

A filesystem can therefore have both:

- gigabytes reported available by `df` or `statfs`; and
- almost no raw space outside already allocated chunks.

If metadata is pressured in that state, ordinary writes can fail with
`ENOSPC`. A balance is not a guaranteed escape hatch because relocation also
needs working space.

`btrfs-headroom` combines the two views instead of replacing one with the
other:

- `statfs` total and available space;
- per-device raw allocated and unallocated space;
- data, metadata, system, and mixed space-info usage;
- metadata commitments such as reserved, pinned, and `bytes_may_use`;
- metadata chunk size and profile cost where the kernel exposes them;
- global reserve use, read-only state, and missing or unwritable devices.

See the upstream
[`btrfs filesystem usage`](https://btrfs.readthedocs.io/en/stable/btrfs-filesystem.html)
and
[`btrfs balance`](https://btrfs.readthedocs.io/en/stable/btrfs-balance.html)
documentation for the underlying allocation model and recovery constraints.

## Safety model

The monitored Btrfs filesystems are **read-only from this program's point of
view**:

- no balance, scrub, trim, defragmentation, deletion, or profile conversion;
- no mutating Btrfs ioctl;
- no invocation of `btrfs` or another repair command;
- no automatic remediation, even at critical health.

Collection uses read-only Btrfs ioctls, read-only sysfs files, a selected
`mountinfo` view, and `statfs`. The process writes only normal output requested
by the caller. The supplied systemd service writes its status under
`/run/btrfs-headroom`.

The project deliberately has no `fix`, `repair`, or `balance` command. Its
`guard` command is only a read-only preflight gate. A warning is evidence to
investigate; it is not authorization to run a generic balance recipe.

## Build

Requirements:

- Linux;
- Go 1.23 or newer;
- a mounted Btrfs filesystem to inspect.

```console
$ make build
$ ./build/bin/btrfs-headroom version
0.1.0-dev
```

Release builds inject their version with Go linker flags. The equivalent local
build is:

```console
$ make build VERSION=0.1.0
$ ./build/bin/btrfs-headroom version
0.1.0
```

No `btrfs-progs` executable is required for collection.

## Usage

With no mount arguments, `btrfs-headroom` discovers Btrfs mounts from
`/proc/self/mountinfo` and deduplicates subvolume mounts by filesystem ID.
Explicit mount arguments limit collection to those paths. `--mountinfo PATH`
selects a different mount table for both discovery and read-only state.

```console
# Human-readable report for every mounted Btrfs filesystem
$ btrfs-headroom scan

# Inspect one filesystem
$ btrfs-headroom scan /

# Atomically write a JSON report
$ btrfs-headroom scan --format json --output /tmp/btrfs-headroom.json /

# Nagios-compatible status and exit code
$ btrfs-headroom check --format nagios /

# Prometheus text exposition on stdout
$ btrfs-headroom scan --format prometheus /

# Observe host mounts from a service with its own mount namespace
$ btrfs-headroom scan --mountinfo /proc/1/mountinfo

# Block a write-heavy operation at CRITICAL or uncertain health
$ btrfs-headroom guard --fail-at critical --unknown block /

# Generate completion for the current shell
$ btrfs-headroom completion zsh
```

Flags must precede mount arguments. Supported formats are:

| Command | Formats | Health affects exit status |
|---|---|---|
| `scan` | `human`, `json`, `prometheus` | No |
| `check` | `human`, `json`, `nagios`, `prometheus` | Yes |
| `guard` | `human`, `json`, `nagios`, `prometheus` | Yes |

`scan` exits `0` after a valid observation regardless of health. It uses `64`
for invalid CLI usage, `69` for collection failure, `73` when the output
cannot be created, and `74` for rendering or output commit failure.

`check` maps the highest observed severity to Nagios codes:

| Code | Meaning |
|---:|---|
| 0 | OK |
| 1 | WARNING |
| 2 | CRITICAL |
| 3 | UNKNOWN or collection unavailable |

Invalid CLI usage and output failures can still return `64`, `73`, or `74`.
Use `scan`, not `check`, for a periodic collector whose service should remain
successful when health becomes warning or critical.

`guard` is an opt-in preflight suitable for wrapping a write-heavy operation:

- `--fail-at critical` (the default) allows WARNING and returns `2` for
  CRITICAL;
- `--fail-at warning` returns `2` for WARNING or CRITICAL;
- `--unknown block` (the default) returns `3` for UNKNOWN, partial confidence,
  or collection failure;
- `--unknown allow` permits uncertainty and collection failure but never
  permits CRITICAL.

`guard` prints a report but has no `--output` option. It does not run the
guarded operation itself and cannot mutate the filesystem.

## Shell completion and manual page

`btrfs-headroom completion bash|zsh|fish` writes a completion script to
standard output. For example:

```console
$ mkdir -p ~/.local/share/zsh/site-functions
$ btrfs-headroom completion zsh \
    > ~/.local/share/zsh/site-functions/_btrfs-headroom
```

The source for `btrfs-headroom(1)` is
[docs/man/btrfs-headroom.1.scd](docs/man/btrfs-headroom.1.scd). Generate and
validate it with `scdoc` and `groff`:

```console
$ make man
$ make man-check
```

Generated documentation and completion files are written under `build/` and
are not committed.

JSON byte counts are decimal strings so consumers do not lose 64-bit integer
precision. The current schema identifiers are
`org.btrfs-headroom.observation/v1` and
`org.btrfs-headroom.report/v1`.

## Policy

The current policy correlates raw device headroom, metadata pressure,
estimated metadata chunk cost, `statfs` availability, global reserve use, and
device state. It is intentionally conservative about incomplete and
multi-device observations.

The v0.1 evaluator is stateless: it does not yet implement trend prediction or
hysteresis. Exact thresholds and reason codes are documented in
[docs/policy.md](docs/policy.md).

## systemd

The supplied system service runs an unprivileged scan every five minutes and
atomically publishes `/run/btrfs-headroom/status.json`:

```console
$ sudo install -Dm0644 contrib/systemd/system/btrfs-headroom.service \
    /usr/lib/systemd/system/btrfs-headroom.service
$ sudo install -Dm0644 contrib/systemd/system/btrfs-headroom.timer \
    /usr/lib/systemd/system/btrfs-headroom.timer
$ sudo install -Dm0644 contrib/sysusers/btrfs-headroom.conf \
    /usr/lib/sysusers.d/btrfs-headroom.conf
$ sudo systemd-sysusers
$ sudo systemctl daemon-reload
$ sudo systemctl enable --now btrfs-headroom.timer
```

Inspect it with:

```console
$ systemctl status btrfs-headroom.timer
$ journalctl -u btrfs-headroom.service
$ jq . /run/btrfs-headroom/status.json
```

The unit uses a dedicated unprivileged system user, an empty capability set, a
private device namespace, and a read-only system view. A static identity keeps
the published runtime directory traversable by local consumers; no login shell
or writable home is assigned. The service reads `/proc/1/mountinfo` so its
hardening namespace is not mistaken for a read-only host filesystem. It never
runs as root. Installation, hardening details, output visibility, and the
optional desktop notifier are covered in
[contrib/systemd/README.md](contrib/systemd/README.md).

## Packaging

Arch package templates are maintained under
[packaging/arch](packaging/arch). They install the binary, systemd units,
notifier helper, documentation, completions, and licenses, but do not enable
either timer or notifier.

Tagged `vMAJOR.MINOR.PATCH` releases publish a Linux amd64 archive containing
the statically linked binary, generated manual page, Bash/Zsh/Fish
completions, README, and licenses. Each release includes `SHA256SUMS` and a
GitHub build-provenance attestation. After downloading an archive, verify it
with:

```console
$ sha256sum --check SHA256SUMS
$ gh attestation verify btrfs-headroom-0.1.0-linux-amd64.tar.gz \
    --repo Mr-Tao/btrfs-headroom
```

Maintainers can reproduce the archive locally with `scdoc`, GNU tar, and:

```console
$ make release VERSION=0.1.0
```

## Scope

`btrfs-headroom` is:

- a focused local check suitable for timers, Nagios, or a Prometheus textfile
  collector;
- an opt-in, read-only preflight gate for external package-management or
  maintenance workflows;
- complementary to `btrfs filesystem usage`;
- independent of maintenance schedulers such as `btrfsmaintenance`.

It is not:

- a filesystem repair tool;
- a replacement for backups, scrub, or device-error monitoring;
- proof that a particular allocation will succeed on every RAID layout;
- a daemon, metrics server, or historical time-series database.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) before sending a change. Report
security-sensitive issues according to [docs/security.md](docs/security.md),
not in a public issue.

## License

Licensed under either Apache License 2.0 or the MIT license, at your option.
See [LICENSE](LICENSE).
