// SPDX-License-Identifier: Apache-2.0 OR MIT

package model

import (
	"encoding/json"
	"math"
	"testing"
)

func TestByteCountMarshalsAsDecimalString(t *testing.T) {
	tests := []struct {
		name  string
		value ByteCount
		want  string
	}{
		{name: "zero", value: 0, want: `"0"`},
		{name: "above JavaScript integer precision", value: 1<<53 + 1, want: `"9007199254740993"`},
		{name: "maximum uint64", value: ByteCount(math.MaxUint64), want: `"18446744073709551615"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("json.Marshal(%d): %v", test.value, err)
			}
			if string(got) != test.want {
				t.Fatalf("json.Marshal(%d) = %s, want %s", test.value, got, test.want)
			}
		})
	}
}
