//go:build darwin

// Package autostart manages the current user's operating-system login item.
package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

const launchAgentFileName = "com.github.zhoux77899.kitsune-proxy.plist"

// Enabled reports whether Kitsune Proxy is registered for the current user.
func Enabled() (bool, error) {
	path, err := launchAgentPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect launch agent: %w", err)
	}
	return true, nil
}

// SetEnabled changes the current user's login startup preference.
func SetEnabled(enabled bool) error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove launch agent: %w", err)
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
	if err := writeEntry(path, launchAgent(executable)); err != nil {
		return fmt.Errorf("write launch agent: %w", err)
	}
	return nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentFileName), nil
}

func launchAgent(executable string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(executable))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.github.zhoux77899.kitsune-proxy</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + escaped.String() + `</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <false/>
</dict>
</plist>
`)
}
