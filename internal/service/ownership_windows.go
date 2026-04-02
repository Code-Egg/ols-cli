//go:build windows

package service

import (
	"fmt"
	"os"
)

func fileOwnership(info os.FileInfo) (int, int, bool) {
	return 0, 0, false
}

func chownPath(path string, uid, gid int) error {
	return nil
}

func lookupUserGroupIDs(userName, groupName string) (int, int, error) {
	return 0, 0, fmt.Errorf("ownership lookup is not supported on windows")
}
