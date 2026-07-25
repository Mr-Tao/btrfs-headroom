<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# systemd integration

The integration has two independent parts:

1. a system timer that collects status without root privileges; and
2. an optional user path unit that sends desktop notifications.

Neither part modifies Btrfs.

## System collector

Install the binary first, then the units:

```console
$ sudo install -Dm0755 btrfs-headroom /usr/bin/btrfs-headroom
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

The timer uses a five-minute wall-clock schedule. `Persistent=yes` causes one
missed scan to be caught up after the host resumes or boots. A short randomized
delay avoids synchronized collection across a fleet.

The oneshot service runs:

```console
$ btrfs-headroom scan --mountinfo /proc/1/mountinfo --format json \
    --output /run/btrfs-headroom/status.json
```

`scan` returns success for every valid health state, so WARNING and CRITICAL
are represented in JSON rather than as a failed systemd unit. Collection or
output failures still fail the service and are recorded in the journal.

The CLI renders into a private temporary file and atomically publishes the
JSON at mode `0644`. The service then touches `status.ready` only after the
report is readable, giving local consumers a path to watch. `UMask=0022` is
intentional for the shared `0644` marker; the user notifier uses `UMask=0077`
for its own state.

Useful commands:

```console
$ systemctl list-timers btrfs-headroom.timer
$ sudo systemctl start btrfs-headroom.service
$ systemctl status btrfs-headroom.service
$ journalctl -u btrfs-headroom.service
$ jq . /run/btrfs-headroom/status.json
```

The default scan discovers all Btrfs mounts. To restrict it, create a drop-in:

```console
$ sudo systemctl edit btrfs-headroom.service
```

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/btrfs-headroom scan --mountinfo /proc/1/mountinfo --format json --output /run/btrfs-headroom/status.json /
```

Flags must precede mountpoints.

## Hardening

The service uses:

- a dedicated `btrfs-headroom` system user with no login shell or writable
  home;
- empty capability and ambient-capability sets;
- `NoNewPrivileges=yes`;
- private device, network, and temporary-filesystem namespaces;
- a read-only system and home view;
- protected kernel, module, log, clock, and control-group interfaces;
- a syscall allowlist that includes the read-only `ioctl` calls needed by the
  collector;
- one writable systemd-managed runtime directory with mode `0755`.

`ProtectSystem=strict` makes the service's own `/` mount appear read-only.
Read-only health must describe the host, not that private mount namespace, so
the unit passes `--mountinfo /proc/1/mountinfo`. It intentionally does not set
`ProtectProc=invisible`, which would hide PID 1 from the service user.
`ProcSubset=pid` remains enabled; the service receives no access to process
memory, block devices, or additional capabilities.

The account is static because systemd deliberately places runtime directories
owned by `DynamicUser=yes` below root-only `/run/private`. That protects
private service state, but it would also make the intentionally shared status
file unreachable by the desktop notifier despite its `0644` mode.

Do not add `User=root`, `CAP_SYS_ADMIN`, access to block devices, or a writable
system view to work around a collection failure. File permissions or an older
kernel should produce an incomplete/failed observation and be fixed narrowly.

The status JSON is intentionally world-readable for unprivileged local
consumers and contains filesystem inventory. See
[../../docs/security.md](../../docs/security.md) before deploying on a
multi-user host.

## Desktop notifier

The optional notifier requires:

- `jq`;
- `notify-send` from `libnotify`;
- a desktop session with a freedesktop notification service.

Install the helper and user units:

```console
$ sudo install -Dm0755 contrib/systemd/user/btrfs-headroom-notify \
    /usr/lib/btrfs-headroom/btrfs-headroom-notify
$ sudo install -Dm0644 contrib/systemd/user/btrfs-headroom-notify.service \
    /usr/lib/systemd/user/btrfs-headroom-notify.service
$ sudo install -Dm0644 contrib/systemd/user/btrfs-headroom-notify.path \
    /usr/lib/systemd/user/btrfs-headroom-notify.path
$ systemctl --user daemon-reload
$ systemctl --user enable --now btrfs-headroom-notify.path
```

To process an already published report immediately:

```console
$ systemctl --user start btrfs-headroom-notify.service
```

The notifier:

- emits nothing for the first healthy report;
- notifies on the first warning, critical, or unknown state;
- replaces its previous notification when severity or the leading reason
  changes;
- emits one recovery notification when health returns to OK;
- stores only a digest, severity, and notification ID below
  `$XDG_RUNTIME_DIR`.

Disable it without affecting collection:

```console
$ systemctl --user disable --now btrfs-headroom-notify.path
```

## Validation

From the repository root:

```console
$ systemd-analyze verify \
    contrib/systemd/system/btrfs-headroom.service \
    contrib/systemd/system/btrfs-headroom.timer
$ systemd-analyze --user verify \
    contrib/systemd/user/btrfs-headroom-notify.service \
    contrib/systemd/user/btrfs-headroom-notify.path
$ shellcheck contrib/systemd/user/btrfs-headroom-notify
```

Some `systemd-analyze verify` versions also check whether the installed
`/usr/bin/btrfs-headroom` and notifier helper exist. Validate against a staged
package or install the files before treating those path errors as unit syntax
failures.
