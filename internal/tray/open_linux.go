//go:build linux

package tray

import "os/exec"

// OpenPath asks the desktop environment to open a file or directory.
func OpenPath(path string) error {
	return exec.Command("xdg-open", path).Start()
}
