// SPDX-License-Identifier: Apache-2.0 OR MIT

package model

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	ObservationSchema = "org.btrfs-headroom.observation/v1"
	ReportSchema      = "org.btrfs-headroom.report/v1"
)

type ByteCount uint64

func (b ByteCount) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatUint(uint64(b), 10))
}

type Collection struct {
	Backend      string   `json:"backend"`
	Completeness string   `json:"completeness"`
	Consistency  string   `json:"consistency"`
	Privilege    string   `json:"privilege"`
	Warnings     []string `json:"warnings,omitempty"`
}

type Filesystem struct {
	FSID          string   `json:"fsid"`
	MetadataUUID  string   `json:"metadata_uuid"`
	Mountpoints   []string `json:"mountpoints"`
	Readonly      bool     `json:"readonly"`
	Nodesize      uint32   `json:"nodesize"`
	Sectorsize    uint32   `json:"sectorsize"`
	NumDevices    uint64   `json:"num_devices"`
	Generation    uint64   `json:"generation"`
	KernelSysfsID string   `json:"kernel_sysfs_id"`
}

type Device struct {
	ID          uint64     `json:"devid"`
	UUID        string     `json:"uuid,omitempty"`
	Path        string     `json:"path,omitempty"`
	Missing     *bool      `json:"missing"`
	Writable    *bool      `json:"writable"`
	Size        *ByteCount `json:"size"`
	Allocated   *ByteCount `json:"allocated"`
	Unallocated *ByteCount `json:"unallocated"`
}

type Profile struct {
	Name         string    `json:"name"`
	Flags        uint64    `json:"flags"`
	LogicalTotal ByteCount `json:"logical_total"`
	LogicalUsed  ByteCount `json:"logical_used"`
}

type SpaceInfo struct {
	Kind              string     `json:"kind"`
	Flags             uint64     `json:"flags"`
	LogicalTotal      ByteCount  `json:"logical_total"`
	LogicalUsed       ByteCount  `json:"logical_used"`
	BytesMayUse       *ByteCount `json:"bytes_may_use"`
	BytesPinned       *ByteCount `json:"bytes_pinned"`
	BytesReadonly     *ByteCount `json:"bytes_readonly"`
	BytesReserved     *ByteCount `json:"bytes_reserved"`
	BytesZoneUnusable *ByteCount `json:"bytes_zone_unusable"`
	ChunkSize         *ByteCount `json:"chunk_size"`
	DiskTotal         *ByteCount `json:"disk_total"`
	DiskUsed          *ByteCount `json:"disk_used"`
	DynamicReclaim    *ByteCount `json:"dynamic_reclaim"`
	Profiles          []Profile  `json:"profiles,omitempty"`
}

type GlobalReserve struct {
	Size      *ByteCount `json:"size"`
	Available *ByteCount `json:"available_reserved"`
	Consumed  *ByteCount `json:"consumed"`
}

type StatFS struct {
	Total     ByteCount `json:"total"`
	Available ByteCount `json:"available"`
}

type Observation struct {
	Schema        string        `json:"schema"`
	CollectedAt   time.Time     `json:"collected_at"`
	Filesystem    Filesystem    `json:"filesystem"`
	Collection    Collection    `json:"collection"`
	Devices       []Device      `json:"devices"`
	SpaceInfos    []SpaceInfo   `json:"space_infos"`
	GlobalReserve GlobalReserve `json:"global_reserve"`
	StatFS        StatFS        `json:"statfs"`
}

type Severity string

const (
	SeverityOK       Severity = "ok"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
	SeverityUnknown  Severity = "unknown"
)

type Reason struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
}

type Health struct {
	Severity   Severity `json:"severity"`
	Confidence string   `json:"confidence"`
	Policy     string   `json:"policy"`
	Reasons    []Reason `json:"reasons"`
}

type Report struct {
	Observation Observation `json:"observation"`
	Health      Health      `json:"health"`
}

type ReportSet struct {
	Schema  string   `json:"schema"`
	Reports []Report `json:"reports"`
}
