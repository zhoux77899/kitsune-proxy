//go:build windows

package tray

import (
	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func platformPresentationFor(status app.Status, label string) platformPresentation {
	health := trayHealthForStatus(status)
	var icon []byte
	switch health {
	case trayHealthy:
		icon = assets.TrayWindowsHealthyICO
	case trayDegraded:
		icon = assets.TrayWindowsDegradedICO
	case trayStopped:
		icon = assets.TrayWindowsStoppedICO
	default:
		icon = assets.TrayWindowsErrorICO
	}
	return platformPresentation{
		icon:    icon,
		iconKey: "windows:" + string(health),
		tooltip: label,
	}
}
