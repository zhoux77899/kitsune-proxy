// Package assets embeds the platform tray icons generated from kitsune.svg.
package assets

import _ "embed"

var (
	// TrayPNG is the generated color PNG used by the Linux tray implementation.
	//go:embed generated/kitsune.png
	TrayPNG []byte

	// TrayWindowsHealthyICO is the generated Windows tray icon for healthy operation.
	//go:embed generated/kitsune-tray-windows-healthy.ico
	TrayWindowsHealthyICO []byte

	// TrayWindowsDegradedICO is the generated Windows tray icon for degraded operation.
	//go:embed generated/kitsune-tray-windows-degraded.ico
	TrayWindowsDegradedICO []byte

	// TrayWindowsErrorICO is the generated Windows tray icon for unavailable operation.
	//go:embed generated/kitsune-tray-windows-error.ico
	TrayWindowsErrorICO []byte

	// TrayWindowsStoppedICO is the generated Windows tray icon for stopped operation.
	//go:embed generated/kitsune-tray-windows-stopped.ico
	TrayWindowsStoppedICO []byte

	// TrayMacOSTemplatePNG is the generated black macOS template image.
	//go:embed generated/kitsune-tray-macos-black.png
	TrayMacOSTemplatePNG []byte

	// AppICNS is the generated macOS application icon.
	//go:embed generated/kitsune.icns
	AppICNS []byte
)
