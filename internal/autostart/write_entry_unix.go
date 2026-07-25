//go:build linux || darwin

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeEntry(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".kitsune-proxy-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(tempPath)
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace entry: %w", err)
	}
	complete = true
	return nil
}
