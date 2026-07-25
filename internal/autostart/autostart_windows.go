//go:build windows

// Package autostart manages the current user's operating-system login item.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "KitsuneProxy"
)

// Enabled reports whether Kitsune Proxy is registered for the current user.
func Enabled() (bool, error) {
	expected, err := commandForExecutable()
	if err != nil {
		return false, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open startup registry key: %w", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read startup registry value: %w", err)
	}
	return value == expected, nil
}

// SetEnabled changes the current user's login startup preference.
func SetEnabled(enabled bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("open startup registry key: %w", err)
		}
		defer key.Close()
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("delete startup registry value: %w", err)
		}
		return nil
	}

	command, err := commandForExecutable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create startup registry key: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(valueName, command); err != nil {
		return fmt.Errorf("write startup registry value: %w", err)
	}
	return nil
}

func commandForExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return quoteWindowsExecutable(executable)
}

func quoteWindowsExecutable(executable string) (string, error) {
	if strings.ContainsRune(executable, '"') {
		return "", errors.New("executable path contains an unsupported quote")
	}
	return `"` + executable + `"`, nil
}
