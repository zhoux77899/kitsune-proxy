//go:build windows

package tray

import "github.com/zhoux77899/kitsune-proxy/assets"

func platformIcon() []byte {
	return assets.TrayICO
}
