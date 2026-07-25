<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Security

## Reporting a vulnerability

Do not open a public issue for a vulnerability, privilege-boundary bypass, or
a report containing sensitive host data. Use GitHub's private security
advisory flow for this repository.

Include the affected revision, operating system and kernel versions, a minimal
reproducer, expected impact, and whether the issue requires a privileged
caller. Remove filesystem UUIDs, device serials, usernames, and private mount
paths unless they are essential to the report.

## Trust model

`btrfs-headroom` treats the kernel, sysfs, mount table, and explicitly supplied
mountpoints as local inputs. It does not parse network data and does not
provide a network service.

The collector:

- opens mountpoints read-only;
- uses `BTRFS_IOC_FS_INFO`, `BTRFS_IOC_DEV_INFO`, and
  `BTRFS_IOC_SPACE_INFO`;
- reads allocation counters below `/sys/fs/btrfs`;
- reads the configured mount table (by default `/proc/self/mountinfo`);
- calls `statfs`;
- does not use a mutating ioctl or execute `btrfs`.

No capability is required by the intended kernel interfaces. Running the
collector as root or granting `CAP_SYS_ADMIN` would unnecessarily enlarge its
impact and is not the supported default.

The read-only claim applies to observed Btrfs filesystems. The CLI can write
the output path named by `--output`, using a temporary file and atomic rename
in the destination directory.

## systemd boundary

The supplied system service uses a dynamic identity and an empty capability
set. It receives a private device namespace and a read-only view of the host
filesystem except for its managed runtime directory. Network access, new
privileges, writable executable memory, kernel tunables, kernel modules,
control groups, and broad namespace creation are restricted.

The unit writes:

```text
/run/btrfs-headroom/status.json
/run/btrfs-headroom/status.ready
```

The system service explicitly reads `/proc/1/mountinfo`. This is necessary
because `ProtectSystem=strict` creates a mount namespace in which `/` appears
read-only to the service. Treating that private view as host state would create
a false `READ_ONLY` critical result. The service therefore does not use
`ProtectProc=invisible`; `ProcSubset=pid` remains enabled and no process memory
or device access is granted.

The runtime directory is managed by systemd with mode `0755`. The system
service intentionally uses `UMask=0022`: the report is rendered through a
private temporary file and atomically published as mode `0644`, and the ready
marker is created as `0644` only after the report is readable. The user
notifier keeps its separate `UMask=0077` for per-user state.

`status.json` can reveal filesystem UUIDs, mountpoints, and device paths. On
systems where that inventory is sensitive, omit the user notifier and replace
the supplied publication mode with an appropriate local access policy.

Runtime status is not authenticated against a privileged local attacker.
Root and processes able to modify `/run` or the service definition are outside
this trust boundary.

## Optional desktop notifier

The notifier is not part of the privileged service. It runs in the user's
systemd manager, reads the published JSON with `jq`, and sends a desktop
notification over the user's D-Bus session through `notify-send`.

It stores only the last severity, content digest, and notification ID beneath
the user's runtime directory. JSON-derived text is passed as command
arguments, not evaluated as shell code.

Enabling the notifier accepts two additional local dependencies (`jq` and
`libnotify`) and exposes the health summary to the desktop notification
daemon. It remains read-only toward Btrfs.

## Non-goals

The project does not attempt to protect against:

- a compromised kernel or local root;
- falsified sysfs or mount namespace data;
- denial of service by inaccessible or rapidly changing mountpoints;
- unsafe recovery commands run independently by an administrator.

An UNKNOWN or partial-confidence result must be handled as uncertainty, not
silently converted to OK.
