// Package assets embeds the platform tray icons generated from kitsune.svg.
package assets

import _ "embed"

var (
	// TrayPNG is the generated PNG used by Linux and macOS tray implementations.
	//go:embed generated/kitsune.png
	TrayPNG []byte

	// TrayICO is the generated multi-resolution Windows tray icon.
	//go:embed generated/kitsune.ico
	TrayICO []byte

	// AppICNS is the generated macOS application icon.
	//go:embed generated/kitsune.icns
	AppICNS []byte
)
