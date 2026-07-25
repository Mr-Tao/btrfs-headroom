<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Contributing

Thank you for helping make Btrfs allocator failures easier to detect before
they become recovery incidents.

## Design constraints

Changes must preserve these project invariants:

1. Collection is read-only with respect to every observed Btrfs filesystem.
2. The project does not automatically run balance, scrub, trim, defrag,
   deletion, profile conversion, or arbitrary repair commands.
3. Incomplete or ambiguous observations must not be presented as confidently
   healthy.
4. Observation data and alert policy remain separate so policy can evolve
   without changing the kernel-data contract.
5. Multi-device and RAID layouts must expose uncertainty rather than infer
   allocatability from an unsafe aggregate.

Proposals for remediation guidance are welcome, but executable repair
subcommands are outside the current scope.

## Before opening a pull request

Open an issue first for schema changes, new policy signals, privilege changes,
or new runtime dependencies. Small documentation, packaging, and bug fixes can
go directly to a pull request.

Keep changes focused. Include:

- the failure mode or observation being addressed;
- kernel and `btrfs-progs` versions where relevant;
- the filesystem profile and number of devices;
- sanitized fixtures or reproduction steps;
- the expected behavior for incomplete and permission-denied collection.

Do not publish filesystem UUIDs, device serials, private mount paths, or
unrelated host data in bug reports.

## Validation

For Go changes:

```console
$ make check
```

For systemd changes:

```console
$ systemd-analyze verify \
    contrib/systemd/system/btrfs-headroom.service \
    contrib/systemd/system/btrfs-headroom.timer
$ systemd-analyze --user verify \
    contrib/systemd/user/btrfs-headroom-notify.service \
    contrib/systemd/user/btrfs-headroom-notify.path
$ shellcheck contrib/systemd/user/btrfs-headroom-notify
```

For Arch packaging changes:

```console
$ bash -n packaging/arch/PKGBUILD
$ namcap packaging/arch/PKGBUILD
```

`namcap` is optional for local development but expected before publishing an
AUR package.

## Tests

Policy changes should include table-driven boundary tests. Collector changes
should include fixtures for missing sysfs attributes, permission failures,
mount changes during collection, and affected RAID profiles. A lower raw
headroom or higher metadata commitment must never improve severity for the
same otherwise identical observation.

Integration tests must use disposable loopback filesystems. Never run a
destructive test against a developer's mounted data filesystem.

## Commits and licensing

Use clear, imperative commit subjects. Sign off commits with:

```console
$ git commit --signoff
```

The sign-off certifies the
[Developer Certificate of Origin 1.1](https://developercertificate.org/).

By contributing, you agree that your contribution is licensed under either
Apache License 2.0 or MIT, at the recipient's option. New source,
configuration, scripts, and documentation should carry:

```text
SPDX-License-Identifier: Apache-2.0 OR MIT
```

Use the appropriate comment syntax for the file format. Do not add the SPDX
line to the verbatim license texts themselves.

## Security reports

Follow [docs/security.md](docs/security.md) for vulnerabilities or reports
that contain sensitive host information.
