//go:build !windows

package cli

import "os"

func hasRootPrivileges() bool {
	return os.Geteuid() == 0
}
