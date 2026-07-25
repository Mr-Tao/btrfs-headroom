// SPDX-License-Identifier: Apache-2.0 OR MIT

package observe

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMountInfoFiltersBtrfsAndDecodesEscapes(t *testing.T) {
	const input = `29 23 8:1 / / rw,relatime - ext4 /dev/sda1 rw
42 29 0:35 /@root /mnt/with\040space\011tab\134slash\012line ro,nosuid,nodev shared:12 - btrfs /dev/sda6 rw,subvolid=256,subvol=/@root
43 29 0:35 /@home /home rw,relatime - btrfs /dev/sda6 rw,subvolid=257,subvol=/@home
`

	got, err := parseMountInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseMountInfo: %v", err)
	}
	want := []mount{
		{path: "/mnt/with space\ttab\\slash\nline", readonly: true, readonlyKnown: true},
		{path: "/home", readonly: false, readonlyKnown: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMountInfo result:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestReadonlyForPathUsesLongestMountpoint(t *testing.T) {
	mounts := []mount{
		{path: "/", readonly: false, readonlyKnown: true},
		{path: "/srv", readonly: true, readonlyKnown: true},
		{path: "/srv/data", readonly: false, readonlyKnown: true},
	}
	tests := []struct {
		path      string
		readonly  bool
		available bool
	}{
		{"/etc", false, true},
		{"/srv/archive", true, true},
		{"/srv/data/file", false, true},
	}
	for _, test := range tests {
		readonly, available := readonlyForPath(test.path, mounts)
		if readonly != test.readonly || available != test.available {
			t.Fatalf("readonlyForPath(%q) = (%v, %v), want (%v, %v)",
				test.path, readonly, available, test.readonly, test.available)
		}
	}
}

func TestParseMountInfoRejectsMalformedLine(t *testing.T) {
	_, err := parseMountInfo(strings.NewReader("42 29 0:35 / / rw\n"))
	if err == nil {
		t.Fatal("parseMountInfo accepted a line without the mountinfo separator")
	}
}

func TestUnescapeMountInfoPreservesInvalidEscape(t *testing.T) {
	const input = `/mnt/bad\999/path`
	if got := unescapeMountInfo(input); got != input {
		t.Fatalf("unescapeMountInfo(%q) = %q, want the invalid escape preserved", input, got)
	}
}
