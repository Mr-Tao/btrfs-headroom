// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build linux

package observe

import (
	"testing"
	"unsafe"
)

func TestBtrfsUAPIStructureSizes(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"fs info", unsafe.Sizeof(fsInfoArgs{}), 1024},
		{"device info", unsafe.Sizeof(devInfoArgs{}), 4096},
		{"space header", unsafe.Sizeof(spaceArgs{}), 16},
		{"space info", unsafe.Sizeof(spaceInfoRaw{}), 24},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("structure size = %d, want %d", test.got, test.want)
			}
		})
	}
}

func TestBtrfsIOCTLNumbers(t *testing.T) {
	if btrfsIOCFSInfo != 0x8400941f {
		t.Fatalf("FS_INFO ioctl = %#x", btrfsIOCFSInfo)
	}
	if btrfsIOCDevInfo != 0xd000941e {
		t.Fatalf("DEV_INFO ioctl = %#x", btrfsIOCDevInfo)
	}
	if btrfsIOCSpaceInfo != 0xc0109414 {
		t.Fatalf("SPACE_INFO ioctl = %#x", btrfsIOCSpaceInfo)
	}
}
