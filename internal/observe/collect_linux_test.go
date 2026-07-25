// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build linux

package observe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

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
