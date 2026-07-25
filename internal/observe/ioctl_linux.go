// SPDX-License-Identifier: Apache-2.0 OR MIT
//
//go:build linux

package observe

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	btrfsIOCFSInfo    = uintptr(0x8400941f)
	btrfsIOCDevInfo   = uintptr(0xd000941e)
	btrfsIOCSpaceInfo = uintptr(0xc0109414)

	fsInfoFlagCsumInfo     = uint64(1 << 0)
	fsInfoFlagGeneration   = uint64(1 << 1)
	fsInfoFlagMetadataUUID = uint64(1 << 2)
)

type fsInfoArgs struct {
	MaxID          uint64
	NumDevices     uint64
	FSID           [16]byte
	Nodesize       uint32
	Sectorsize     uint32
	CloneAlignment uint32
	CsumType       uint16
	CsumSize       uint16
	Flags          uint64
	Generation     uint64
	MetadataUUID   [16]byte
	Reserved       [944]byte
}

type devInfoArgs struct {
	Devid      uint64
	UUID       [16]byte
	BytesUsed  uint64
	TotalBytes uint64
	FSID       [16]byte
	Unused     [377]uint64
	Path       [1024]byte
}

type spaceArgs struct {
	SpaceSlots uint64
	Total      uint64
}

type spaceInfoRaw struct {
	Flags      uint64
	TotalBytes uint64
	UsedBytes  uint64
}

func callIOCTL(fd uintptr, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	runtime.KeepAlive(arg)
	if errno != 0 {
		return errno
	}
	return nil
}

func getFSInfo(fd uintptr) (fsInfoArgs, error) {
	info := fsInfoArgs{
		Flags: fsInfoFlagCsumInfo | fsInfoFlagGeneration | fsInfoFlagMetadataUUID,
	}
	if unsafe.Sizeof(info) != 1024 {
		return info, fmt.Errorf("unexpected FS_INFO structure size: %d", unsafe.Sizeof(info))
	}
	if err := callIOCTL(fd, btrfsIOCFSInfo, unsafe.Pointer(&info)); err != nil {
		if !errors.Is(err, syscall.EINVAL) {
			return info, fmt.Errorf("BTRFS_IOC_FS_INFO: %w", err)
		}
		info = fsInfoArgs{}
		if fallbackErr := callIOCTL(fd, btrfsIOCFSInfo, unsafe.Pointer(&info)); fallbackErr != nil {
			return info, fmt.Errorf("BTRFS_IOC_FS_INFO compatibility retry: %w", fallbackErr)
		}
	}
	return info, nil
}

func getDevices(fd uintptr, fsInfo fsInfoArgs) ([]devInfoArgs, []string) {
	var devices []devInfoArgs
	var warnings []string
	if fsInfo.MaxID > 1_000_000 {
		return nil, []string{fmt.Sprintf("implausible maximum device id %d", fsInfo.MaxID)}
	}
	for devid := uint64(1); devid <= fsInfo.MaxID; devid++ {
		info := devInfoArgs{Devid: devid}
		if unsafe.Sizeof(info) != 4096 {
			return nil, []string{fmt.Sprintf("unexpected DEV_INFO structure size: %d", unsafe.Sizeof(info))}
		}
		err := callIOCTL(fd, btrfsIOCDevInfo, unsafe.Pointer(&info))
		if errors.Is(err, syscall.ENODEV) {
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("BTRFS_IOC_DEV_INFO devid %d: %v", devid, err))
			continue
		}
		devices = append(devices, info)
		if uint64(len(devices)) >= fsInfo.NumDevices {
			break
		}
	}
	if uint64(len(devices)) != fsInfo.NumDevices {
		warnings = append(warnings, fmt.Sprintf(
			"kernel reported %d devices but %d were readable",
			fsInfo.NumDevices, len(devices),
		))
	}
	return devices, warnings
}

func getSpaceInfo(fd uintptr) ([]spaceInfoRaw, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var count spaceArgs
		if err := callIOCTL(fd, btrfsIOCSpaceInfo, unsafe.Pointer(&count)); err != nil {
			return nil, fmt.Errorf("BTRFS_IOC_SPACE_INFO count: %w", err)
		}
		if count.Total > 4096 {
			return nil, fmt.Errorf("implausible space-info count %d", count.Total)
		}
		if count.Total == 0 {
			return nil, nil
		}

		headerSize := unsafe.Sizeof(spaceArgs{})
		infoSize := unsafe.Sizeof(spaceInfoRaw{})
		raw := make([]byte, headerSize+uintptr(count.Total)*infoSize)
		header := (*spaceArgs)(unsafe.Pointer(&raw[0]))
		header.SpaceSlots = count.Total
		if err := callIOCTL(fd, btrfsIOCSpaceInfo, unsafe.Pointer(&raw[0])); err != nil {
			return nil, fmt.Errorf("BTRFS_IOC_SPACE_INFO data: %w", err)
		}
		if header.Total > header.SpaceSlots {
			continue
		}

		result := make([]spaceInfoRaw, 0, header.Total)
		for i := uint64(0); i < header.Total; i++ {
			offset := headerSize + uintptr(i)*infoSize
			info := *(*spaceInfoRaw)(unsafe.Add(unsafe.Pointer(&raw[0]), offset))
			result = append(result, info)
		}
		runtime.KeepAlive(raw)
		return result, nil
	}
	return nil, errors.New("BTRFS_IOC_SPACE_INFO changed during three collection attempts")
}

func cString(data []byte) string {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	return string(data)
}
