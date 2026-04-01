//go:build !windows

package service

import (
	"os"
	"syscall"
)

func fileOwnership(info os.FileInfo) (int, int, bool) {
	if info == nil {
		return 0, 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}

func chownPath(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}
