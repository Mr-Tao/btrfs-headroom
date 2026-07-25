// SPDX-License-Identifier: Apache-2.0 OR MIT

package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

func Write(w io.Writer, format string, reports []model.Report) error {
	switch format {
	case "human":
		return writeHuman(w, reports)
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(model.ReportSet{Schema: model.ReportSchema, Reports: reports})
	case "nagios":
		return writeNagios(w, reports)
	case "prometheus":
		return writePrometheus(w, reports)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeHuman(w io.Writer, reports []model.Report) error {
	for i, report := range reports {
		if i > 0 {
			fmt.Fprintln(w)
		}
		observation := report.Observation
		fmt.Fprintf(w, "%s %s\n", strings.ToUpper(string(report.Health.Severity)), observation.Filesystem.FSID)
		fmt.Fprintf(w, "  mounts: %s\n", strings.Join(observation.Filesystem.Mountpoints, ", "))
		if unallocated, ok := sumUnallocated(observation.Devices); ok {
			fmt.Fprintf(w, "  raw unallocated: %s\n", formatBytes(unallocated))
		} else {
			fmt.Fprintln(w, "  raw unallocated: unknown")
		}
		fmt.Fprintf(w, "  statfs available: %s\n", formatBytes(uint64(observation.StatFS.Available)))
		if pressure, ok := metadataPressure(observation.SpaceInfos); ok {
			fmt.Fprintf(w, "  metadata pressure: %.1f%%\n", pressure*100)
		}
		for _, reason := range report.Health.Reasons {
			fmt.Fprintf(w, "  - %s: %s\n", reason.Code, reason.Summary)
		}
	}
	return nil
}

func writeNagios(w io.Writer, reports []model.Report) error {
	severity := highestSeverity(reports)
	var summaries []string
	for _, report := range reports {
		if report.Health.Severity != model.SeverityOK {
			summaries = append(summaries,
				fmt.Sprintf("%s=%s", report.Observation.Filesystem.FSID, report.Health.Severity))
		}
	}
	if len(summaries) == 0 {
		summaries = append(summaries, fmt.Sprintf("%d filesystem(s) healthy", len(reports)))
	}
	_, err := fmt.Fprintf(w, "BTRFS_HEADROOM %s - %s\n",
		strings.ToUpper(string(severity)), strings.Join(summaries, ", "))
	return err
}

func writePrometheus(w io.Writer, reports []model.Report) error {
	var buffer bytes.Buffer
	buffer.WriteString("# HELP btrfs_headroom_device_unallocated_bytes Raw device space outside allocated chunks.\n")
	buffer.WriteString("# TYPE btrfs_headroom_device_unallocated_bytes gauge\n")
	buffer.WriteString("# HELP btrfs_headroom_statfs_available_bytes Space reported available by statfs.\n")
	buffer.WriteString("# TYPE btrfs_headroom_statfs_available_bytes gauge\n")
	buffer.WriteString("# HELP btrfs_headroom_metadata_pressure_ratio Metadata commitments divided by allocated metadata space.\n")
	buffer.WriteString("# TYPE btrfs_headroom_metadata_pressure_ratio gauge\n")
	buffer.WriteString("# HELP btrfs_headroom_health Health state: 0 OK, 1 UNKNOWN, 2 WARNING, 3 CRITICAL.\n")
	buffer.WriteString("# TYPE btrfs_headroom_health gauge\n")
	buffer.WriteString("# HELP btrfs_headroom_collector_success Whether the observation was complete.\n")
	buffer.WriteString("# TYPE btrfs_headroom_collector_success gauge\n")
	for _, report := range reports {
		fsid := prometheusQuote(report.Observation.Filesystem.FSID)
		if unallocated, ok := sumUnallocated(report.Observation.Devices); ok {
			fmt.Fprintf(&buffer, "btrfs_headroom_device_unallocated_bytes{fsid=%q} %d\n",
				fsid, unallocated)
		}
		fmt.Fprintf(&buffer, "btrfs_headroom_statfs_available_bytes{fsid=%q} %d\n",
			fsid, report.Observation.StatFS.Available)
		if pressure, ok := metadataPressure(report.Observation.SpaceInfos); ok {
			fmt.Fprintf(&buffer, "btrfs_headroom_metadata_pressure_ratio{fsid=%q} %.8f\n", fsid, pressure)
		}
		fmt.Fprintf(&buffer, "btrfs_headroom_health{fsid=%q} %d\n",
			fsid, severityRank(report.Health.Severity))
		success := 0
		if report.Observation.Collection.Completeness == "complete" {
			success = 1
		}
		fmt.Fprintf(&buffer, "btrfs_headroom_collector_success{fsid=%q} %d\n", fsid, success)
	}
	_, err := io.Copy(w, &buffer)
	return err
}

func highestSeverity(reports []model.Report) model.Severity {
	severity := model.SeverityOK
	for _, report := range reports {
		if severityRank(report.Health.Severity) > severityRank(severity) {
			severity = report.Health.Severity
		}
	}
	return severity
}

func ExitCode(reports []model.Report) int {
	switch highestSeverity(reports) {
	case model.SeverityWarning:
		return 1
	case model.SeverityCritical:
		return 2
	case model.SeverityUnknown:
		return 3
	default:
		return 0
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

func sumUnallocated(devices []model.Device) (uint64, bool) {
	if len(devices) == 0 {
		return 0, false
	}
	var value uint64
	for _, device := range devices {
		if device.Unallocated == nil {
			return 0, false
		}
		value += uint64(*device.Unallocated)
	}
	return value, true
}

func metadataPressure(spaces []model.SpaceInfo) (float64, bool) {
	for _, space := range spaces {
		if space.Kind != "metadata" && space.Kind != "mixed" {
			continue
		}
		if space.LogicalTotal == 0 {
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
				value += uint64(*extra)
			}
		}
		return float64(value) / float64(space.LogicalTotal), true
	}
	return 0, false
}

func formatBytes(value uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)
	switch {
	case value >= tib:
		return fmt.Sprintf("%.2f TiB", float64(value)/tib)
	case value >= gib:
		return fmt.Sprintf("%.2f GiB", float64(value)/gib)
	case value >= mib:
		return fmt.Sprintf("%.2f MiB", float64(value)/mib)
	case value >= kib:
		return fmt.Sprintf("%.2f KiB", float64(value)/kib)
	default:
		return fmt.Sprintf("%d B", value)
	}
}

func prometheusQuote(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return replacer.Replace(value)
}

func SortReasons(reports []model.Report) {
	for i := range reports {
		sort.SliceStable(reports[i].Health.Reasons, func(a, b int) bool {
			return reports[i].Health.Reasons[a].Code < reports[i].Health.Reasons[b].Code
		})
	}
}
