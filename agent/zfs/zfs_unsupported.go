//go:build !linux

package zfs

import "errors"

func ARCSize() (uint64, error) {
	return 0, errors.New("ZFS is only supported on Linux")
}
