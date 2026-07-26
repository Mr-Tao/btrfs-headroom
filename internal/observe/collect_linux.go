// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build linux

package observe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

const (
	blockGroupData     = uint64(1 << 0)
	blockGroupSystem   = uint64(1 << 1)
	blockGroupMetadata = uint64(1 << 2)
	blockGroupRAID0    = uint64(1 << 3)
	blockGroupRAID1    = uint64(1 << 4)
	blockGroupDUP      = uint64(1 << 5)
	blockGroupRAID10   = uint64(1 << 6)
	blockGroupRAID5    = uint64(1 << 7)
	blockGroupRAID6    = uint64(1 << 8)
	blockGroupRAID1C3  = uint64(1 << 9)
	blockGroupRAID1C4  = uint64(1 << 10)
	spaceInfoGlobalRSV = uint64(1 << 49)
)

type Collector struct {
	MountInfoPath string
	SysfsRoot     string
	Now           func() time.Time
}

func NewCollector() Collector {
	return Collector{
		MountInfoPath: "/proc/self/mountinfo",
		SysfsRoot:     "/sys/fs/btrfs",
		Now:           time.Now,
	}
}

func (c Collector) Collect(paths []string) ([]model.Observation, error) {
	mounts, err := c.mounts(paths)
	if err != nil {
		return nil, err
	}
	if len(mounts) == 0 {
		return nil, errors.New("no Btrfs mounts found")
	}
	return collectObservations(mounts, c.collectMount)
}

func collectObservations(
	mounts []mount,
	collect func(mount) (model.Observation, error),
) ([]model.Observation, error) {
	byFSID := make(map[string]*model.Observation)
	for _, mount := range mounts {
		observation, err := collect(mount)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mount.path, err)
		}
		fsid := observation.Filesystem.FSID
		if existing, ok := byFSID[fsid]; ok {
			existing.Filesystem.Mountpoints = append(existing.Filesystem.Mountpoints, mount.path)
			existing.Filesystem.Readonly = existing.Filesystem.Readonly && observation.Filesystem.Readonly
			continue
		}
		byFSID[fsid] = &observation
	}

	result := make([]model.Observation, 0, len(byFSID))
	for _, observation := range byFSID {
		sort.Strings(observation.Filesystem.Mountpoints)
		result = append(result, *observation)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filesystem.FSID < result[j].Filesystem.FSID
	})
	return result, nil
}

func (c Collector) mounts(paths []string) ([]mount, error) {
	knownMounts, mountInfoErr := c.readMounts()
	if len(paths) > 0 {
		result := make([]mount, 0, len(paths))
		for _, path := range paths {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", path, err)
			}
			resolved := mount{path: absolute}
			if mountInfoErr == nil {
				resolved.readonly, resolved.readonlyKnown = readonlyForPath(absolute, knownMounts)
			}
			result = append(result, resolved)
		}
		return result, nil
	}
	if mountInfoErr != nil {
		return nil, mountInfoErr
	}
	return knownMounts, nil
}

func (c Collector) readMounts() ([]mount, error) {
	file, err := os.Open(c.MountInfoPath)
	if err != nil {
		return nil, fmt.Errorf("open mountinfo: %w", err)
	}
	defer file.Close()
	return parseMountInfo(file)
}

func (c Collector) collectMount(mount mount) (model.Observation, error) {
	file, err := os.Open(mount.path)
	if err != nil {
		return model.Observation{}, fmt.Errorf("open mountpoint: %w", err)
	}
	defer file.Close()

	fsInfo, err := getFSInfo(file.Fd())
	if err != nil {
		return model.Observation{}, err
	}
	fsid := formatUUID(fsInfo.FSID)
	metadataUUID := formatUUID(fsInfo.MetadataUUID)
	if isZeroUUID(fsInfo.MetadataUUID) {
		metadataUUID = fsid
	}
	sysfsID := c.resolveSysfsID(fsid, metadataUUID)
	observation := model.Observation{
		Schema:      model.ObservationSchema,
		CollectedAt: c.Now().UTC(),
		Filesystem: model.Filesystem{
			FSID:          fsid,
			MetadataUUID:  metadataUUID,
			Mountpoints:   []string{mount.path},
			Readonly:      mount.readonly,
			Nodesize:      fsInfo.Nodesize,
			Sectorsize:    fsInfo.Sectorsize,
			NumDevices:    fsInfo.NumDevices,
			Generation:    fsInfo.Generation,
			KernelSysfsID: sysfsID,
		},
		Collection: model.Collection{
			Backend:      "linux-uapi+sysfs+statfs",
			Completeness: "complete",
			Consistency:  "best-effort",
			Privilege:    privilege(),
		},
	}
	if !mount.readonlyKnown {
		observation.Collection.Warnings = append(
			observation.Collection.Warnings,
			"mount read-only state is unavailable",
		)
	}

	rawDevices, warnings := getDevices(file.Fd(), fsInfo)
	observation.Collection.Warnings = append(observation.Collection.Warnings, warnings...)
	for _, raw := range rawDevices {
		allocated := raw.BytesUsed
		if allocated > raw.TotalBytes {
			observation.Collection.Warnings = append(
				observation.Collection.Warnings,
				fmt.Sprintf("device %d allocated bytes exceed its size", raw.Devid),
			)
			allocated = raw.TotalBytes
		}
		sizeValue := model.ByteCount(raw.TotalBytes)
		allocatedValue := model.ByteCount(allocated)
		unallocatedValue := model.ByteCount(raw.TotalBytes - allocated)
		missingValue := false
		writableValue := true
		device := model.Device{
			ID:          raw.Devid,
			UUID:        formatUUID(raw.UUID),
			Path:        cString(raw.Path[:]),
			Missing:     &missingValue,
			Writable:    &writableValue,
			Size:        &sizeValue,
			Allocated:   &allocatedValue,
			Unallocated: &unallocatedValue,
		}
		c.readDeviceState(sysfsID, &device)
		observation.Devices = append(observation.Devices, device)
	}
	observation.Devices = c.appendSysfsDevices(sysfsID, observation.Devices, &observation.Collection)
	sort.Slice(observation.Devices, func(i, j int) bool {
		return observation.Devices[i].ID < observation.Devices[j].ID
	})

	rawSpaces, err := getSpaceInfo(file.Fd())
	if err != nil {
		return model.Observation{}, err
	}
	var ioctlGlobalReserve *spaceInfoRaw
	spaceIndexes := make(map[string]int)
	for _, raw := range rawSpaces {
		if raw.Flags&spaceInfoGlobalRSV != 0 {
			value := raw
			ioctlGlobalReserve = &value
			continue
		}
		kind := spaceKind(raw.Flags)
		profile := model.Profile{
			Name:         profileName(raw.Flags),
			Flags:        raw.Flags,
			LogicalTotal: model.ByteCount(raw.TotalBytes),
			LogicalUsed:  model.ByteCount(raw.UsedBytes),
		}
		if index, ok := spaceIndexes[kind]; ok {
			space := &observation.SpaceInfos[index]
			space.Flags |= raw.Flags
			space.LogicalTotal += model.ByteCount(raw.TotalBytes)
			space.LogicalUsed += model.ByteCount(raw.UsedBytes)
			space.Profiles = append(space.Profiles, profile)
			continue
		}
		spaceIndexes[kind] = len(observation.SpaceInfos)
		observation.SpaceInfos = append(observation.SpaceInfos, model.SpaceInfo{
			Kind:         kind,
			Flags:        raw.Flags,
			LogicalTotal: model.ByteCount(raw.TotalBytes),
			LogicalUsed:  model.ByteCount(raw.UsedBytes),
			Profiles:     []model.Profile{profile},
		})
	}
	c.readSpaceAllocations(sysfsID, &observation)
	sort.Slice(observation.SpaceInfos, func(i, j int) bool {
		return observation.SpaceInfos[i].Kind < observation.SpaceInfos[j].Kind
	})

	observation.GlobalReserve = c.readGlobalReserve(sysfsID)
	if ioctlGlobalReserve != nil {
		if observation.GlobalReserve.Size == nil {
			size := model.ByteCount(ioctlGlobalReserve.TotalBytes)
			observation.GlobalReserve.Size = &size
		}
		if observation.GlobalReserve.Consumed == nil {
			consumed := model.ByteCount(ioctlGlobalReserve.UsedBytes)
			observation.GlobalReserve.Consumed = &consumed
		}
		if observation.GlobalReserve.Available == nil {
			available := model.ByteCount(0)
			if ioctlGlobalReserve.TotalBytes > ioctlGlobalReserve.UsedBytes {
				available = model.ByteCount(ioctlGlobalReserve.TotalBytes - ioctlGlobalReserve.UsedBytes)
			}
			observation.GlobalReserve.Available = &available
		}
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mount.path, &stat); err != nil {
		return model.Observation{}, fmt.Errorf("statfs: %w", err)
	}
	blockSize := uint64(stat.Bsize)
	observation.StatFS = model.StatFS{
		Total:     model.ByteCount(stat.Blocks * blockSize),
		Available: model.ByteCount(stat.Bavail * blockSize),
	}
	if len(observation.Collection.Warnings) > 0 {
		observation.Collection.Completeness = "partial"
	}
	return observation, nil
}

func (c Collector) readSpaceAllocations(fsid string, observation *model.Observation) {
	for index := range observation.SpaceInfos {
		space := &observation.SpaceInfos[index]
		c.readAllocation(fsid, space)
		if requiredAllocationCountersMissing(space) {
			observation.Collection.Warnings = append(
				observation.Collection.Warnings,
				fmt.Sprintf("modern sysfs allocation counters unavailable for %s", space.Kind),
			)
			observation.Collection.Completeness = "partial"
		}
	}
}

func (c Collector) readAllocation(fsid string, space *model.SpaceInfo) {
	root := filepath.Join(c.SysfsRoot, fsid, "allocation", space.Kind)
	space.BytesMayUse = readByteCount(filepath.Join(root, "bytes_may_use"))
	space.BytesPinned = readByteCount(filepath.Join(root, "bytes_pinned"))
	space.BytesReadonly = readByteCount(filepath.Join(root, "bytes_readonly"))
	space.BytesReserved = readByteCount(filepath.Join(root, "bytes_reserved"))
	space.BytesZoneUnusable = readByteCount(filepath.Join(root, "bytes_zone_unusable"))
	space.ChunkSize = readByteCount(filepath.Join(root, "chunk_size"))
	space.DiskTotal = readByteCount(filepath.Join(root, "disk_total"))
	space.DiskUsed = readByteCount(filepath.Join(root, "disk_used"))
	space.DynamicReclaim = readByteCount(filepath.Join(root, "dynamic_reclaim"))
}

func requiredAllocationCountersMissing(space *model.SpaceInfo) bool {
	switch space.Kind {
	case "data", "metadata", "mixed", "system":
		return space.BytesMayUse == nil || space.ChunkSize == nil
	default:
		return false
	}
}

func (c Collector) resolveSysfsID(fsid, metadataUUID string) string {
	for _, candidate := range []string{fsid, metadataUUID} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(c.SysfsRoot, candidate)); err == nil && info.IsDir() {
			return candidate
		}
	}
	entries, err := os.ReadDir(c.SysfsRoot)
	if err != nil {
		return fsid
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range []string{"metadata_uuid", "temp_fsid"} {
			data, err := os.ReadFile(filepath.Join(c.SysfsRoot, entry.Name(), name))
			if err == nil && (strings.TrimSpace(string(data)) == fsid ||
				strings.TrimSpace(string(data)) == metadataUUID) {
				return entry.Name()
			}
		}
	}
	return fsid
}

func (c Collector) readGlobalReserve(fsid string) model.GlobalReserve {
	root := filepath.Join(c.SysfsRoot, fsid, "allocation")
	size := readByteCount(filepath.Join(root, "global_rsv_size"))
	available := readByteCount(filepath.Join(root, "global_rsv_reserved"))
	var consumed *model.ByteCount
	if size != nil && available != nil {
		value := model.ByteCount(0)
		if *size > *available {
			value = *size - *available
		}
		consumed = &value
	}
	return model.GlobalReserve{
		Size:      size,
		Available: available,
		Consumed:  consumed,
	}
}

func (c Collector) readDeviceState(fsid string, device *model.Device) {
	root := filepath.Join(c.SysfsRoot, fsid, "devinfo", strconv.FormatUint(device.ID, 10))
	if value, ok := readBool(filepath.Join(root, "missing")); ok {
		device.Missing = &value
	}
	if value, ok := readBool(filepath.Join(root, "writeable")); ok {
		device.Writable = &value
	}
}

func (c Collector) appendSysfsDevices(
	fsid string,
	devices []model.Device,
	collection *model.Collection,
) []model.Device {
	known := make(map[uint64]bool, len(devices))
	for _, device := range devices {
		known[device.ID] = true
	}
	root := filepath.Join(c.SysfsRoot, fsid, "devinfo")
	entries, err := os.ReadDir(root)
	if err != nil {
		return devices
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		devid, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil || known[devid] {
			continue
		}
		missing, missingKnown := readBool(filepath.Join(root, entry.Name(), "missing"))
		writable, writableKnown := readBool(filepath.Join(root, entry.Name(), "writeable"))
		device := model.Device{ID: devid}
		if missingKnown {
			device.Missing = &missing
		}
		if writableKnown {
			device.Writable = &writable
		}
		devices = append(devices, device)
		collection.Warnings = append(
			collection.Warnings,
			fmt.Sprintf("device %d is visible in sysfs but DEV_INFO is unavailable", devid),
		)
	}
	return devices
}

func readByteCount(path string) *model.ByteCount {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return nil
	}
	result := model.ByteCount(value)
	return &result
}

func readBool(path string) (bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		return true, true
	case "0":
		return false, true
	default:
		return false, false
	}
}

func privilege() string {
	if os.Geteuid() == 0 {
		return "root"
	}
	return "unprivileged"
}

func formatUUID(uuid [16]byte) string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
	)
}

func isZeroUUID(uuid [16]byte) bool {
	for _, value := range uuid {
		if value != 0 {
			return false
		}
	}
	return true
}

func spaceKind(flags uint64) string {
	switch flags & (blockGroupData | blockGroupMetadata | blockGroupSystem) {
	case blockGroupData:
		return "data"
	case blockGroupMetadata:
		return "metadata"
	case blockGroupSystem:
		return "system"
	case blockGroupData | blockGroupMetadata:
		return "mixed"
	default:
		return "unknown"
	}
}

func profileName(flags uint64) string {
	switch {
	case flags&blockGroupRAID1C4 != 0:
		return "raid1c4"
	case flags&blockGroupRAID1C3 != 0:
		return "raid1c3"
	case flags&blockGroupRAID6 != 0:
		return "raid6"
	case flags&blockGroupRAID5 != 0:
		return "raid5"
	case flags&blockGroupRAID10 != 0:
		return "raid10"
	case flags&blockGroupDUP != 0:
		return "dup"
	case flags&blockGroupRAID1 != 0:
		return "raid1"
	case flags&blockGroupRAID0 != 0:
		return "raid0"
	default:
		return "single"
	}
}
