btrfs-headroom(1) "btrfs-headroom" "General Commands Manual"
; SPDX-License-Identifier: Apache-2.0 OR MIT

# NAME

btrfs-headroom - inspect Btrfs allocator headroom without modifying filesystems

# SYNOPSIS

*btrfs-headroom scan* [*--format* _FORMAT_] [*--output* _PATH_]
\[*--mountinfo* _PATH_] [_MOUNT_...]

*btrfs-headroom check* [*--format* _FORMAT_] [*--output* _PATH_]
\[*--mountinfo* _PATH_] [_MOUNT_...]

*btrfs-headroom guard* [*--format* _FORMAT_] [*--fail-at* _LEVEL_]
\[*--unknown* _POLICY_] [*--mountinfo* _PATH_] [_MOUNT_...]

*btrfs-headroom completion* _SHELL_

*btrfs-headroom version*

# DESCRIPTION

*btrfs-headroom* is a read-only health check for the Btrfs allocator. It
combines logical filesystem availability with raw unallocated device space,
space-info usage, metadata commitments, chunk cost, reserve use, and device
state. This detects conditions where *df*(1) still reports free space but
Btrfs may be unable to allocate another metadata block group.

With no _MOUNT_ arguments, the program discovers Btrfs mounts through
_/proc/self/mountinfo_ and deduplicates subvolume mounts by filesystem ID.
Explicit mount arguments limit collection to those paths. Options must precede
mount arguments.

The program never runs balance, scrub, trim, defragmentation, deletion,
profile conversion, or another repair command. It uses read-only Btrfs ioctls,
sysfs, mount information, and *statfs*(2). Only an explicitly requested output
file or the program's own service state is written.

# COMMANDS

*scan*
	Produce a report. Health does not affect a successful command's exit
	status. This is suitable for periodic collection.

*check*
	Produce a report and map the highest observed health severity to a
	Nagios-compatible exit status.

*guard*
	Produce a report and act as an opt-in, read-only preflight gate for an
	external operation. The program does not execute the guarded operation.

*completion* _bash|zsh|fish_
	Write a completion script for the selected shell to standard output.

*version*
	Print the version embedded at build time.

*help*
	Print command usage.

# SCAN AND CHECK OPTIONS

*--format* _FORMAT_
	Select *human*, *json*, or *prometheus*. The *check* command additionally
	accepts *nagios*. The default is *human*.

*--output* _PATH_
	Write the report atomically to _PATH_. Use *-* for standard output, which
	is the default.

*--mountinfo* _PATH_
	Read mount discovery and read-only state from _PATH_. The default is
	_/proc/self/mountinfo_.

# GUARD OPTIONS

*--format* _FORMAT_
	Select *human*, *json*, *nagios*, or *prometheus*. The default is
	*human*.

*--fail-at* _LEVEL_
	Select *warning* or *critical*. At *warning*, either WARNING or CRITICAL
	blocks the external operation. At the default *critical*, WARNING is
	allowed and CRITICAL blocks it.

*--unknown* _POLICY_
	Select *block* or *allow*. The default *block* rejects UNKNOWN,
	incomplete observations, and collection failures. *allow* permits
	uncertainty and collection failure but never permits CRITICAL health.

*--mountinfo* _PATH_
	Read mount discovery and read-only state from _PATH_. The default is
	_/proc/self/mountinfo_.

# OUTPUT

Human output is intended for interactive use. JSON uses the schemas
*org.btrfs-headroom.observation/v1* and
*org.btrfs-headroom.report/v1*. JSON byte counts are decimal strings to
preserve 64-bit integer precision. Prometheus output is suitable for a
textfile collector. Nagios output summarizes the highest observed severity.

# EXIT STATUS

*0*
	A valid *scan* completed; *check* observed OK; or *guard* allowed the
	external operation.

*1*
	*check* observed WARNING.

*2*
	*check* observed CRITICAL, or *guard* blocked WARNING or CRITICAL.

*3*
	*check* observed UNKNOWN or collection was unavailable, or *guard*
	blocked an incomplete or unknown observation.

*64*
	Command-line usage was invalid.

*69*
	*scan* could not collect an observation.

*73*
	An output file could not be created.

*74*
	A report, completion script, or output file could not be completed.

# EXAMPLES

Inspect all discovered Btrfs filesystems:

```
btrfs-headroom scan
```

Atomically publish a JSON report for the root filesystem:

```
btrfs-headroom scan --format json --output /tmp/btrfs-headroom.json /
```

Use a conservative preflight before a write-heavy operation:

```
btrfs-headroom guard --fail-at critical --unknown block /
```

Install Zsh completion for the current user:

```
mkdir -p ~/.local/share/zsh/site-functions
btrfs-headroom completion zsh > \
	~/.local/share/zsh/site-functions/_btrfs-headroom
```

# FILES

_/proc/self/mountinfo_
	Default mount discovery and read-only state source.

_/sys/fs/btrfs_
	Kernel allocator and device state.

_/run/btrfs-headroom/status.json_
	Default report published by the supplied systemd service.

# SEE ALSO

*btrfs*(8), *btrfs-filesystem*(8), *btrfs-balance*(8), *df*(1), *statfs*(2)

Project documentation: _https://github.com/Mr-Tao/btrfs-headroom_
