//go:build windows

package service

import "os"

func fileOwnership(info os.FileInfo) (int, int, bool) {
	return 0, 0, false
}

func chownPath(path string, uid, gid int) error {
	return nil
}
