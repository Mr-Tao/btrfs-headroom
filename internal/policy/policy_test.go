// SPDX-License-Identifier: Apache-2.0 OR MIT

package policy

import (
	"testing"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

const testGiB = uint64(1024 * 1024 * 1024)

func TestEvaluateReportsLowUnallocatedHeadroom(t *testing.T) {
	observation := healthyObservation()
	unallocated := model.ByteCount(testGiB)
	observation.Devices[0].Unallocated = &unallocated

	health := Evaluate(observation)

	if health.Severity != model.SeverityWarning {
		t.Fatalf("severity = %q, want %q; reasons: %#v",
			health.Severity, model.SeverityWarning, health.Reasons)
	}
	reason := findReason(health, "LOW_UNALLOCATED")
	if reason == nil {
		t.Fatalf("LOW_UNALLOCATED reason missing: %#v", health.Reasons)
	}
	if reason.Severity != model.SeverityWarning {
		t.Fatalf("LOW_UNALLOCATED severity = %q, want %q",
			reason.Severity, model.SeverityWarning)
	}
}

func TestEvaluateReportsCriticalMetadataPressure(t *testing.T) {
	observation := healthyObservation()
	metadata := &observation.SpaceInfos[0]
	metadata.LogicalUsed = model.ByteCount(uint64(metadata.LogicalTotal) * 98 / 100)

	health := Evaluate(observation)

	if health.Severity != model.SeverityCritical {
		t.Fatalf("severity = %q, want %q; reasons: %#v",
			health.Severity, model.SeverityCritical, health.Reasons)
	}
	reason := findReason(health, "METADATA_PRESSURE")
	if reason == nil {
		t.Fatalf("METADATA_PRESSURE reason missing: %#v", health.Reasons)
	}
	if reason.Severity != model.SeverityCritical {
		t.Fatalf("METADATA_PRESSURE severity = %q, want %q",
			reason.Severity, model.SeverityCritical)
	}
}

func TestSeverityDoesNotImproveAsRawHeadroomFalls(t *testing.T) {
	headrooms := []uint64{
		8 * testGiB,
		4 * testGiB,
		2 * testGiB,
		testGiB,
		testGiB / 2,
		0,
	}

	for _, pressurePercent := range []uint64{50, 95} {
		t.Run(pressureName(pressurePercent), func(t *testing.T) {
			previousRank := -1
			for _, headroom := range headrooms {
				observation := healthyObservation()
				unallocated := model.ByteCount(headroom)
				observation.Devices[0].Unallocated = &unallocated
				metadata := &observation.SpaceInfos[0]
				metadata.LogicalUsed = model.ByteCount(
					uint64(metadata.LogicalTotal) * pressurePercent / 100,
				)

				health := Evaluate(observation)
				rank := testSeverityRank(health.Severity)
				if rank < previousRank {
					t.Fatalf(
						"severity improved from rank %d to %d when headroom fell to %d bytes; reasons: %#v",
						previousRank, rank, headroom, health.Reasons,
					)
				}
				previousRank = rank
			}
		})
	}
}

func TestSeverityDoesNotImproveAsMetadataPressureRises(t *testing.T) {
	pressures := []uint64{0, 50, 89, 90, 96, 97, 100}
	previousRank := -1

	for _, pressurePercent := range pressures {
		observation := healthyObservation()
		metadata := &observation.SpaceInfos[0]
		metadata.LogicalUsed = model.ByteCount(
			uint64(metadata.LogicalTotal) * pressurePercent / 100,
		)

		health := Evaluate(observation)
		rank := testSeverityRank(health.Severity)
		if rank < previousRank {
			t.Fatalf(
				"severity improved from rank %d to %d when metadata pressure rose to %d%%; reasons: %#v",
				previousRank, rank, pressurePercent, health.Reasons,
			)
		}
		previousRank = rank
	}
}

func TestMultipleMetadataProfilesAreUnknown(t *testing.T) {
	observation := healthyObservation()
	observation.SpaceInfos[0].Profiles = append(
		observation.SpaceInfos[0].Profiles,
		model.Profile{Name: "dup"},
	)

	health := Evaluate(observation)

	if health.Confidence != "partial" {
		t.Fatalf("confidence = %q, want partial", health.Confidence)
	}
	if reason := findReason(health, "MULTIPLE_METADATA_PROFILES"); reason == nil {
		t.Fatalf("MULTIPLE_METADATA_PROFILES reason missing: %#v", health.Reasons)
	}
}

func TestUnknownDeviceHeadroomDoesNotProveChunkInfeasibility(t *testing.T) {
	tests := map[string]func(*model.Device){
		"missing values": func(device *model.Device) {
			device.Size = nil
			device.Allocated = nil
			device.Unallocated = nil
		},
		"zero size": func(device *model.Device) {
			zero := model.ByteCount(0)
			device.Size = &zero
			device.Allocated = &zero
			device.Unallocated = &zero
		},
	}
	for name, makeUnknown := range tests {
		t.Run(name, func(t *testing.T) {
			observation := healthyObservation()
			makeUnknown(&observation.Devices[0])
			metadata := &observation.SpaceInfos[0]
			metadata.LogicalUsed = model.ByteCount(uint64(metadata.LogicalTotal) * 95 / 100)

			health := Evaluate(observation)

			if health.Severity != model.SeverityWarning {
				t.Fatalf("severity = %q, want %q; reasons: %#v",
					health.Severity, model.SeverityWarning, health.Reasons)
			}
			if reason := findReason(health, "DEVICE_HEADROOM_UNKNOWN"); reason == nil {
				t.Fatalf("DEVICE_HEADROOM_UNKNOWN reason missing: %#v", health.Reasons)
			}
			if reason := findReason(health, "NO_METADATA_CHUNK_HEADROOM"); reason != nil {
				t.Fatalf("unknown headroom produced NO_METADATA_CHUNK_HEADROOM: %#v", reason)
			}
		})
	}
}

func TestMissingDeviceIsCritical(t *testing.T) {
	observation := healthyObservation()
	missing := true
	notWritable := false
	observation.Devices = append(observation.Devices, model.Device{
		ID:       2,
		Missing:  &missing,
		Writable: &notWritable,
	})

	health := Evaluate(observation)

	if health.Severity != model.SeverityCritical {
		t.Fatalf("severity = %q, want critical; reasons: %#v", health.Severity, health.Reasons)
	}
	if reason := findReason(health, "MISSING_DEVICE"); reason == nil {
		t.Fatalf("MISSING_DEVICE reason missing: %#v", health.Reasons)
	}
	if reason := findReason(health, "DEVICE_HEADROOM_UNKNOWN"); reason == nil {
		t.Fatalf("DEVICE_HEADROOM_UNKNOWN reason missing: %#v", health.Reasons)
	}
}

func healthyObservation() model.Observation {
	chunkSize := model.ByteCount(testGiB)
	deviceSize := model.ByteCount(100 * testGiB)
	allocated := model.ByteCount(92 * testGiB)
	unallocated := model.ByteCount(8 * testGiB)
	missing := false
	writable := true
	return model.Observation{
		Collection: model.Collection{
			Completeness: "complete",
		},
		Filesystem: model.Filesystem{},
		Devices: []model.Device{{
			ID:          1,
			Missing:     &missing,
			Writable:    &writable,
			Size:        &deviceSize,
			Allocated:   &allocated,
			Unallocated: &unallocated,
		}},
		SpaceInfos: []model.SpaceInfo{{
			Kind:         "metadata",
			LogicalTotal: model.ByteCount(10 * testGiB),
			LogicalUsed:  model.ByteCount(5 * testGiB),
			ChunkSize:    &chunkSize,
			Profiles: []model.Profile{{
				Name: "single",
			}},
		}},
		StatFS: model.StatFS{
			Total:     model.ByteCount(100 * testGiB),
			Available: model.ByteCount(30 * testGiB),
		},
	}
}

func findReason(health model.Health, code string) *model.Reason {
	for i := range health.Reasons {
		if health.Reasons[i].Code == code {
			return &health.Reasons[i]
		}
	}
	return nil
}

func testSeverityRank(severity model.Severity) int {
	switch severity {
	case model.SeverityUnknown:
		return 1
	case model.SeverityWarning:
		return 2
	case model.SeverityCritical:
		return 3
	default:
		return 0
	}
}

func pressureName(percent uint64) string {
	if percent >= 90 {
		return "high-pressure"
	}
	return "low-pressure"
}
