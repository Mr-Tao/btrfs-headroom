// SPDX-License-Identifier: Apache-2.0 OR MIT

package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

const renderGiB = uint64(1024 * 1024 * 1024)

func TestWriteJSON(t *testing.T) {
	report := renderReport("11111111-2222-3333-4444-555555555555", model.SeverityWarning)
	var output bytes.Buffer

	if err := Write(&output, "json", []model.Report{report}); err != nil {
		t.Fatalf("Write(json): %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("rendered JSON is invalid: %v\n%s", err, output.String())
	}
	if got := document["schema"]; got != model.ReportSchema {
		t.Fatalf("schema = %#v, want %q", got, model.ReportSchema)
	}
	reports, ok := document["reports"].([]any)
	if !ok || len(reports) != 1 {
		t.Fatalf("reports = %#v, want one report", document["reports"])
	}
	renderedReport := reports[0].(map[string]any)
	observation := renderedReport["observation"].(map[string]any)
	devices := observation["devices"].([]any)
	device := devices[0].(map[string]any)
	if got, want := device["unallocated"], "3221225472"; got != want {
		t.Fatalf("device unallocated = %#v, want decimal JSON string %q", got, want)
	}
	statfs := observation["statfs"].(map[string]any)
	if got, want := statfs["available"], "5368709120"; got != want {
		t.Fatalf("statfs available = %#v, want decimal JSON string %q", got, want)
	}
}

func TestWriteNagiosUsesHighestSeverity(t *testing.T) {
	reports := []model.Report{
		renderReport("healthy-fsid", model.SeverityOK),
		renderReport("critical-fsid", model.SeverityCritical),
	}
	var output bytes.Buffer

	if err := Write(&output, "nagios", reports); err != nil {
		t.Fatalf("Write(nagios): %v", err)
	}
	const want = "BTRFS_HEADROOM CRITICAL - critical-fsid=critical\n"
	if output.String() != want {
		t.Fatalf("Nagios output = %q, want %q", output.String(), want)
	}
	if got := ExitCode(reports); got != 2 {
		t.Fatalf("ExitCode = %d, want 2", got)
	}
}

func TestExitCodeMapsNagiosSeverities(t *testing.T) {
	tests := []struct {
		severity model.Severity
		want     int
	}{
		{severity: model.SeverityOK, want: 0},
		{severity: model.SeverityWarning, want: 1},
		{severity: model.SeverityCritical, want: 2},
		{severity: model.SeverityUnknown, want: 3},
	}
	for _, test := range tests {
		t.Run(string(test.severity), func(t *testing.T) {
			report := renderReport("test-fsid", test.severity)
			if got := ExitCode([]model.Report{report}); got != test.want {
				t.Fatalf("ExitCode(%s) = %d, want %d", test.severity, got, test.want)
			}
		})
	}
}

func TestWritePrometheus(t *testing.T) {
	report := renderReport("11111111-2222-3333-4444-555555555555", model.SeverityWarning)
	var output bytes.Buffer

	if err := Write(&output, "prometheus", []model.Report{report}); err != nil {
		t.Fatalf("Write(prometheus): %v", err)
	}

	required := []string{
		"# TYPE btrfs_headroom_device_unallocated_bytes gauge\n",
		`btrfs_headroom_device_unallocated_bytes{fsid="11111111-2222-3333-4444-555555555555"} 3221225472` + "\n",
		`btrfs_headroom_statfs_available_bytes{fsid="11111111-2222-3333-4444-555555555555"} 5368709120` + "\n",
		`btrfs_headroom_metadata_pressure_ratio{fsid="11111111-2222-3333-4444-555555555555"} 0.95000000` + "\n",
		`btrfs_headroom_health{fsid="11111111-2222-3333-4444-555555555555"} 2` + "\n",
		`btrfs_headroom_collector_success{fsid="11111111-2222-3333-4444-555555555555"} 1` + "\n",
	}
	for _, fragment := range required {
		if !strings.Contains(output.String(), fragment) {
			t.Errorf("Prometheus output is missing %q:\n%s", fragment, output.String())
		}
	}
}

func renderReport(fsid string, severity model.Severity) model.Report {
	mayUse := model.ByteCount(renderGiB / 2)
	deviceSize := model.ByteCount(100 * renderGiB)
	allocated := model.ByteCount(97 * renderGiB)
	unallocated := model.ByteCount(3 * renderGiB)
	return model.Report{
		Observation: model.Observation{
			Schema: model.ObservationSchema,
			Filesystem: model.Filesystem{
				FSID:        fsid,
				Mountpoints: []string{"/"},
			},
			Collection: model.Collection{
				Completeness: "complete",
			},
			Devices: []model.Device{{
				ID:          1,
				Size:        &deviceSize,
				Allocated:   &allocated,
				Unallocated: &unallocated,
			}},
			SpaceInfos: []model.SpaceInfo{{
				Kind:         "metadata",
				LogicalTotal: model.ByteCount(10 * renderGiB),
				LogicalUsed:  model.ByteCount(9 * renderGiB),
				BytesMayUse:  &mayUse,
			}},
			StatFS: model.StatFS{
				Total:     model.ByteCount(100 * renderGiB),
				Available: model.ByteCount(5 * renderGiB),
			},
		},
		Health: model.Health{
			Severity: severity,
			Reasons: []model.Reason{{
				Code:     "TEST_REASON",
				Severity: severity,
				Summary:  "test summary",
			}},
		},
	}
}
