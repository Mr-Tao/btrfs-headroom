// SPDX-License-Identifier: Apache-2.0 OR MIT

package observe

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

type mount struct {
	path          string
	readonly      bool
	readonlyKnown bool
}

func parseMountInfo(r io.Reader) ([]mount, error) {
	var mounts []mount
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+1 >= len(fields) {
			return nil, fmt.Errorf("malformed mountinfo line")
		}
		if fields[separator+1] != "btrfs" {
			continue
		}
		options := "," + fields[5] + ","
		mounts = append(mounts, mount{
			path:          unescapeMountInfo(fields[4]),
			readonly:      strings.Contains(options, ",ro,"),
			readonlyKnown: true,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read mountinfo: %w", err)
	}
	return mounts, nil
}

func unescapeMountInfo(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '\\' && i+3 < len(value) {
			if decoded, err := strconv.ParseUint(value[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(decoded))
				i += 4
				continue
			}
		}
		b.WriteByte(value[i])
		i++
	}
	return b.String()
}

func readonlyForPath(path string, mounts []mount) (bool, bool) {
	bestLength := -1
	readonly := false
	for _, candidate := range mounts {
		relative, err := filepath.Rel(candidate.path, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(candidate.path) > bestLength {
			bestLength = len(candidate.path)
			readonly = candidate.readonly
		}
	}
	return readonly, bestLength >= 0
}
