//go:build linux

// Package autostart manages the current user's operating-system login item.
package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const desktopFileName = "kitsune-proxy.desktop"

// Enabled reports whether Kitsune Proxy is registered for the current user.
func Enabled() (bool, error) {
	path, err := desktopFilePath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect autostart entry: %w", err)
	}
	return true, nil
}

// SetEnabled changes the current user's login startup preference.
func SetEnabled(enabled bool) error {
	path, err := desktopFilePath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove autostart entry: %w", err)
		}
		return nil
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	content := []byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Kitsune Proxy\n" +
		"Exec=" + quoteDesktopExecutable(executable) + "\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n")
	if err := writeEntry(path, content); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}
	return nil
}

func desktopFilePath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" || !filepath.IsAbs(configHome) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "autostart", desktopFileName), nil
}

func quoteDesktopExecutable(executable string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"`", "\\`",
		"$", `\$`,
		"%", "%%",
	)
	return `"` + replacer.Replace(executable) + `"`
}
