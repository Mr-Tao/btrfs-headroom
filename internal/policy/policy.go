// SPDX-License-Identifier: Apache-2.0 OR MIT

package policy

import (
	"fmt"
	"math"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

const (
	gib = uint64(1024 * 1024 * 1024)
	mib = uint64(1024 * 1024)
)

func Evaluate(observation model.Observation) model.Health {
	health := model.Health{
		Severity:   model.SeverityOK,
		Confidence: "full",
		Policy:     "default-v1",
		Reasons:    []model.Reason{},
	}
	if observation.Collection.Completeness != "complete" {
		health.Confidence = "partial"
		addReason(&health, model.SeverityUnknown, "INCOMPLETE_OBSERVATION",
			"Some allocator values could not be collected")
	}
	if len(observation.Devices) > 1 {
		health.Confidence = "partial"
		addReason(&health, model.SeverityUnknown, "MULTI_DEVICE_FEASIBILITY_UNKNOWN",
			"Per-device chunk placement is not yet proven for multi-device profiles")
	}
	if observation.Filesystem.Readonly {
		addReason(&health, model.SeverityCritical, "READ_ONLY",
			"Filesystem is mounted read-only")
	}

	var deviceSize uint64
	var unallocated uint64
	deviceHeadroomKnown := len(observation.Devices) > 0
	for _, device := range observation.Devices {
		if device.Size == nil || device.Unallocated == nil || *device.Size == 0 {
			deviceHeadroomKnown = false
		} else {
			deviceSize += uint64(*device.Size)
			unallocated += uint64(*device.Unallocated)
		}
		if device.Missing != nil && *device.Missing {
			addReason(&health, model.SeverityCritical, "MISSING_DEVICE",
				fmt.Sprintf("Device %d is missing", device.ID))
		}
		if device.Writable != nil && !*device.Writable {
			addReason(&health, model.SeverityCritical, "UNWRITABLE_DEVICE",
				fmt.Sprintf("Device %d is not writable", device.ID))
		}
	}

	metadata := findMetadata(observation.SpaceInfos)
	if metadata != nil && len(metadata.Profiles) > 1 {
		health.Confidence = "partial"
		addReason(&health, model.SeverityUnknown, "MULTIPLE_METADATA_PROFILES",
			"Metadata uses multiple allocation profiles; next-chunk feasibility is transitional")
	}
	metadataPressure, pressureKnown := pressure(metadata)
	rawChunkCost := metadataChunkRawCost(metadata)
	warningHeadroom := uint64(2 * gib)
	if rawChunkCost > 0 && rawChunkCost <= math.MaxUint64/2 {
		warningHeadroom = max(warningHeadroom, rawChunkCost*2)
	}
	if deviceHeadroomKnown && unallocated < warningHeadroom {
		addReason(&health, model.SeverityWarning, "LOW_UNALLOCATED",
			fmt.Sprintf("Raw unallocated headroom is %s; warning threshold is %s",
				formatBytes(unallocated), formatBytes(warningHeadroom)))
	}
	if pressureKnown {
		switch {
		case metadataPressure >= 0.97:
			addReason(&health, model.SeverityCritical, "METADATA_PRESSURE",
				fmt.Sprintf("Metadata pressure is %.1f%%", metadataPressure*100))
		case metadataPressure >= 0.90:
			addReason(&health, model.SeverityWarning, "METADATA_PRESSURE",
				fmt.Sprintf("Metadata pressure is %.1f%%", metadataPressure*100))
		}
		if deviceHeadroomKnown &&
			rawChunkCost > 0 &&
			unallocated < rawChunkCost &&
			metadataPressure >= 0.90 {
			addReason(&health, model.SeverityCritical, "NO_METADATA_CHUNK_HEADROOM",
				fmt.Sprintf("Raw headroom cannot fund the estimated %s next metadata chunk",
					formatBytes(rawChunkCost)))
		}
	}

	total := uint64(observation.StatFS.Total)
	available := uint64(observation.StatFS.Available)
	if total > 0 {
		warning := clamp(total/20, 2*gib, 20*gib)
		critical := clamp(total/100, 512*mib, 5*gib)
		switch {
		case available < critical:
			addReason(&health, model.SeverityCritical, "LOW_STATFS_AVAILABLE",
				fmt.Sprintf("statfs available space is %s", formatBytes(available)))
		case available < warning:
			addReason(&health, model.SeverityWarning, "LOW_STATFS_AVAILABLE",
				fmt.Sprintf("statfs available space is %s", formatBytes(available)))
		}
	}

	if observation.GlobalReserve.Consumed != nil && *observation.GlobalReserve.Consumed > 0 {
		severity := model.SeverityWarning
		if observation.GlobalReserve.Size != nil &&
			uint64(*observation.GlobalReserve.Size) > 0 &&
			uint64(*observation.GlobalReserve.Consumed)*10 >= uint64(*observation.GlobalReserve.Size) {
			severity = model.SeverityCritical
		}
		addReason(&health, severity, "GLOBAL_RESERVE_CONSUMED",
			fmt.Sprintf("Global reserve consumption is %s",
				formatBytes(uint64(*observation.GlobalReserve.Consumed))))
	}

	if !deviceHeadroomKnown || deviceSize == 0 {
		addReason(&health, model.SeverityUnknown, "DEVICE_HEADROOM_UNKNOWN",
			"Per-device raw headroom is unavailable")
	}
	return health
}

func findMetadata(spaces []model.SpaceInfo) *model.SpaceInfo {
	for i := range spaces {
		if spaces[i].Kind == "metadata" || spaces[i].Kind == "mixed" {
			return &spaces[i]
		}
	}
	return nil
}

func pressure(space *model.SpaceInfo) (float64, bool) {
	if space == nil || space.LogicalTotal == 0 {
		return 0, false
	}
	value := uint64(space.LogicalUsed)
	for _, extra := range []*model.ByteCount{
		space.BytesMayUse,
		space.BytesPinned,
		space.BytesReadonly,
		space.BytesReserved,
		space.BytesZoneUnusable,
	} {
		if extra != nil {
			if math.MaxUint64-value < uint64(*extra) {
				return 1, true
			}
			value += uint64(*extra)
		}
	}
	return float64(value) / float64(space.LogicalTotal), true
}

func metadataChunkRawCost(space *model.SpaceInfo) uint64 {
	if space == nil || space.ChunkSize == nil {
		return 0
	}
	copies := uint64(1)
	for _, profile := range space.Profiles {
		switch profile.Name {
		case "dup", "raid1", "raid10":
			copies = max(copies, 2)
		case "raid1c3":
			copies = max(copies, 3)
		case "raid1c4":
			copies = max(copies, 4)
		}
	}
	return uint64(*space.ChunkSize) * copies
}

func addReason(health *model.Health, severity model.Severity, code, summary string) {
	health.Reasons = append(health.Reasons, model.Reason{
		Code:     code,
		Severity: severity,
		Summary:  summary,
	})
	if severityRank(severity) > severityRank(health.Severity) {
		health.Severity = severity
	}
}

func severityRank(severity model.Severity) int {
	switch severity {
	case model.SeverityCritical:
		return 3
	case model.SeverityWarning:
		return 2
	case model.SeverityUnknown:
		return 1
	default:
		return 0
	}
}

func clamp(value, minimum, maximum uint64) uint64 {
	return min(max(value, minimum), maximum)
}

func formatBytes(value uint64) string {
	const (
		kib = uint64(1024)
		tib = gib * 1024
	)
	switch {
	case value >= tib:
		return fmt.Sprintf("%.2f TiB", float64(value)/float64(tib))
	case value >= gib:
		return fmt.Sprintf("%.2f GiB", float64(value)/float64(gib))
	case value >= mib:
		return fmt.Sprintf("%.2f MiB", float64(value)/float64(mib))
	case value >= kib:
		return fmt.Sprintf("%.2f KiB", float64(value)/float64(kib))
	default:
		return fmt.Sprintf("%d B", value)
	}
}
