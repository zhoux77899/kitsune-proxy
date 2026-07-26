//go:build darwin

package tray

import (
	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func platformPresentationFor(_ app.Status, _ string) platformPresentation {
	return platformPresentation{
		icon:     assets.TrayMacOSTemplatePNG,
		iconKey:  "macos:template",
		template: true,
	}
}
