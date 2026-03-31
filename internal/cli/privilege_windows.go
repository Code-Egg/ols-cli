//go:build windows

package cli

func hasRootPrivileges() bool {
	// Windows is not a target runtime for this CLI, but keep builds portable.
	return true
}
