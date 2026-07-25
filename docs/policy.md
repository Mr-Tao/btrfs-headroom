<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Health policy

This document describes the policy implemented by the current
`0.1.0-dev` CLI. These are project defaults, not universal guarantees made by
Btrfs upstream.

## Inputs

The evaluator consumes one observation per filesystem:

- filesystem read-only state from the selected `mountinfo` source;
- device size, raw allocated and unallocated bytes, missing state, and
  writability;
- logical data, metadata, system, or mixed block-group allocation;
- sysfs commitments: `bytes_may_use`, pinned, read-only, reserved, and zoned
  unusable bytes;
- metadata chunk size and block-group profile;
- `statfs` total and available bytes;
- global reserve size and consumption;
- collection completeness.

The collector deduplicates mountpoints by FSID. Byte counts in JSON are
decimal strings.

## Severity ordering

When several reasons apply, the report uses the highest severity:

```text
critical > warning > unknown > ok
```

For multiple filesystems, `check` returns the highest report severity.
Confidence is independent of severity. The current evaluator reports
`confidence=partial` for an incomplete observation and for every multi-device
filesystem.

## Preflight guard

`guard` evaluates the same observation and policy without launching another
command or modifying Btrfs.

With the defaults, `--fail-at critical --unknown block`:

- returns `2` for CRITICAL;
- permits WARNING;
- returns `3` for UNKNOWN, partial confidence, an unknown reason, or collection
  failure;
- returns `0` only when the configured gate permits the observation.

`--fail-at warning` also maps WARNING to exit `2`. `--unknown allow` maps
uncertainty and collection failure to success, but CRITICAL still returns `2`.
This explicit opt-in is intended for callers that have their own failure
policy.

## Metadata pressure

Metadata pressure is:

```text
logical_used
+ bytes_may_use
+ bytes_pinned
+ bytes_readonly
+ bytes_reserved
+ bytes_zone_unusable
--------------------------------
logical_total
```

Only counters exposed by the running kernel are included. The global reserve
is not added again because its commitments are represented by the metadata
accounting.

| Condition | Severity | Reason code |
|---|---|---|
| pressure >= 90% | warning | `METADATA_PRESSURE` |
| pressure >= 97% | critical | `METADATA_PRESSURE` |

## Raw allocator headroom

Raw headroom is the sum of `device total - device allocated` for observed
devices. It is physical space outside existing Btrfs chunks, not the same
quantity as `df` availability.

The estimated raw cost of the next metadata chunk is:

```text
metadata chunk size * estimated profile copies
```

The current copy estimate is one for single, two for DUP/RAID1/RAID10, three
for RAID1C3, and four for RAID1C4.

The warning threshold is:

```text
max(2 GiB, 2 * estimated next metadata chunk raw cost)
```

| Condition | Severity | Reason code |
|---|---|---|
| raw unallocated < warning threshold | warning | `LOW_UNALLOCATED` |
| metadata pressure >= 90% and raw unallocated < one estimated metadata chunk | critical | `NO_METADATA_CHUNK_HEADROOM` |

`LOW_UNALLOCATED` alone is not critical. Chunk feasibility is more complex
than an aggregate sum, especially for multi-device RAID layouts. The current
release marks those reports partial rather than claiming a precise per-device
allocation result.

## `statfs` availability

The warning threshold is 5% of total logical size, clamped to the range
2-20 GiB. The critical threshold is 1%, clamped to 512 MiB-5 GiB.

| Condition | Severity | Reason code |
|---|---|---|
| available < warning threshold | warning | `LOW_STATFS_AVAILABLE` |
| available < critical threshold | critical | `LOW_STATFS_AVAILABLE` |

These checks complement raw headroom. Neither quantity is substituted for the
other.

## Global reserve

| Condition | Severity | Reason code |
|---|---|---|
| consumed > 0 | warning | `GLOBAL_RESERVE_CONSUMED` |
| consumed >= 10% of reserve size | critical | `GLOBAL_RESERVE_CONSUMED` |

If the running kernel does not expose enough data to calculate consumption,
this rule is not evaluated.

## Device and filesystem state

| Condition | Severity | Reason code |
|---|---|---|
| filesystem is mounted read-only | critical | `READ_ONLY` |
| an observed device is missing | critical | `MISSING_DEVICE` |
| an observed device is not writable | critical | `UNWRITABLE_DEVICE` |
| no usable per-device size/headroom data | unknown | `DEVICE_HEADROOM_UNKNOWN` |
| collection reports omitted values | unknown | `INCOMPLETE_OBSERVATION` |

A warning or critical condition outranks an unknown reason. The unknown reason
is still retained in JSON so consumers can see the reduced confidence.

Read-only state deliberately does not come from the collector process's
`statfs` mount flags. Hardening such as `ProtectSystem=strict` gives a service
a read-only private mount view even when the host filesystem remains writable.
The default CLI reads `/proc/self/mountinfo`; a namespaced service should pass
an authoritative host view such as:

```console
$ btrfs-headroom scan --mountinfo /proc/1/mountinfo
```

If the selected mount table cannot establish the state for an explicit path,
the observation is partial instead of assuming writable.

## Stateless behavior

The current evaluator processes one snapshot. It has no history, trend,
hysteresis, or "N of M samples" rule. Periodic consumers should therefore
expect a report near a threshold to change state between runs.

Future stateful alerting may debounce notifications, but it must not alter the
underlying observation or hide a hard condition such as read-only state or a
missing device.

## Interpretation

A critical result means that continued write-heavy work deserves immediate
attention. It does not prove that every next write will fail, and it does not
select a repair operation.

Before remediation, preserve evidence and inspect both:

```console
$ df -hT /mountpoint
$ sudo btrfs filesystem usage -T /mountpoint
```

Consult upstream recovery guidance for the exact filesystem layout. Do not
turn a generic threshold into an unbounded balance command.
