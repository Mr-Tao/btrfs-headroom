// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build linux

package observe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

func TestCollectObservationsRejectsPartialAutodiscovery(t *testing.T) {
	mounts := []mount{{path: "/healthy"}, {path: "/unreadable"}}
	collect := func(candidate mount) (model.Observation, error) {
		if candidate.path == "/unreadable" {
			return model.Observation{}, errors.New("permission denied")
		}
		return model.Observation{
			Filesystem: model.Filesystem{
				FSID:        "healthy-fsid",
				Mountpoints: []string{candidate.path},
			},
		}, nil
	}

	observations, err := collectObservations(mounts, collect)
	if err == nil {
		t.Fatal("collectObservations accepted a partial autodiscovery result")
	}
	if observations != nil {
		t.Fatalf("partial observations returned: %#v", observations)
	}
	if !strings.Contains(err.Error(), "/unreadable: permission denied") {
		t.Fatalf("error = %q, want failing mountpoint and cause", err)
	}
}

func TestAppendSysfsMissingDevice(t *testing.T) {
	root := t.TempDir()
	deviceRoot := filepath.Join(root, "test-fsid", "devinfo", "2")
	if err := os.MkdirAll(deviceRoot, 0o755); err != nil {
		t.Fatalf("mkdir sysfs fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deviceRoot, "missing"), []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write missing fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deviceRoot, "writeable"), []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write writeable fixture: %v", err)
	}

	collector := Collector{SysfsRoot: root}
	collection := model.Collection{}
	devices := collector.appendSysfsDevices(
		"test-fsid",
		[]model.Device{{ID: 1}},
		&collection,
	)

	if len(devices) != 2 {
		t.Fatalf("device count = %d, want 2", len(devices))
	}
	missing := devices[1]
	if missing.ID != 2 || missing.Missing == nil || !*missing.Missing {
		t.Fatalf("missing device = %#v", missing)
	}
	if missing.Writable == nil || *missing.Writable {
		t.Fatalf("missing writable state = %#v", missing.Writable)
	}
	if missing.Size != nil || missing.Allocated != nil || missing.Unallocated != nil {
		t.Fatalf("unknown missing-device sizes must be null: %#v", missing)
	}
	if len(collection.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one", collection.Warnings)
	}
}

func TestReadAllocationUsesMixedDirectory(t *testing.T) {
	root := t.TempDir()
	allocationRoot := filepath.Join(root, "test-fsid", "allocation", "mixed")
	if err := os.MkdirAll(allocationRoot, 0o755); err != nil {
		t.Fatalf("mkdir mixed allocation fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(allocationRoot, "bytes_may_use"),
		[]byte("4096\n"),
		0o644,
	); err != nil {
		t.Fatalf("write bytes_may_use fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(allocationRoot, "chunk_size"),
		[]byte("268435456\n"),
		0o644,
	); err != nil {
		t.Fatalf("write chunk_size fixture: %v", err)
	}

	collector := Collector{SysfsRoot: root}
	space := model.SpaceInfo{Kind: "mixed"}
	collector.readAllocation("test-fsid", &space)

	if space.BytesMayUse == nil || uint64(*space.BytesMayUse) != 4096 {
		t.Fatalf("mixed bytes_may_use = %#v, want 4096", space.BytesMayUse)
	}
	if space.ChunkSize == nil || uint64(*space.ChunkSize) != 268435456 {
		t.Fatalf("mixed chunk_size = %#v, want 268435456", space.ChunkSize)
	}
	if requiredAllocationCountersMissing(&space) {
		t.Fatalf("complete mixed counters reported missing: %#v", space)
	}
}

func TestMissingMixedAllocationCountersAreIncomplete(t *testing.T) {
	collector := Collector{SysfsRoot: t.TempDir()}
	observation := model.Observation{
		Collection: model.Collection{Completeness: "complete"},
		SpaceInfos: []model.SpaceInfo{{Kind: "mixed"}},
	}

	collector.readSpaceAllocations("test-fsid", &observation)

	if observation.Collection.Completeness != "partial" {
		t.Fatalf("completeness = %q, want partial", observation.Collection.Completeness)
	}
	if len(observation.Collection.Warnings) != 1 ||
		!strings.Contains(observation.Collection.Warnings[0], "unavailable for mixed") {
		t.Fatalf("warnings = %#v, want missing mixed counters", observation.Collection.Warnings)
	}
}
