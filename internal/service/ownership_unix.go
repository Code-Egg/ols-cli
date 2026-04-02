//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
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

func lookupUserGroupIDs(userName, groupName string) (int, int, error) {
	userName = strings.TrimSpace(userName)
	groupName = strings.TrimSpace(groupName)
	if userName == "" {
		return 0, 0, fmt.Errorf("OpenLiteSpeed user directive is empty")
	}

	u, err := user.Lookup(userName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to lookup user %q: %w", userName, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid uid for user %q: %w", userName, err)
	}

	gid := 0
	if groupName == "" {
		gid, err = strconv.Atoi(u.Gid)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid primary gid for user %q: %w", userName, err)
		}
		return uid, gid, nil
	}

	g, err := user.LookupGroup(groupName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to lookup group %q: %w", groupName, err)
	}
	gid, err = strconv.Atoi(g.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid gid for group %q: %w", groupName, err)
	}
	return uid, gid, nil
}
