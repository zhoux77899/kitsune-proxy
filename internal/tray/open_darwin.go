//go:build darwin

package tray

import "os/exec"

// OpenPath asks Finder to open a file or directory.
func OpenPath(path string) error {
	return exec.Command("open", path).Start()
}
