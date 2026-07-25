// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build !linux

package observe

import (
	"errors"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

type Collector struct{}

func NewCollector() Collector {
	return Collector{}
}

func (Collector) Collect([]string) ([]model.Observation, error) {
	return nil, errors.New("btrfs-headroom is supported only on Linux")
}
