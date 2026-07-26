//go:build !windows && !darwin

package tray

import (
	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func platformPresentationFor(_ app.Status, label string) platformPresentation {
	return platformPresentation{
		icon:    assets.TrayPNG,
		iconKey: "unix:color",
		title:   label,
		tooltip: label,
	}
}
