//go:build windows

package tray

import (
	"bytes"
	"testing"

	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func TestWindowsPresentationSelectsHealthIconAndTooltip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status app.Status
		key    string
		icon   []byte
	}{
		{
			name: "healthy",
			status: app.Status{
				State:            app.StateRunning,
				Address:          "http://127.0.0.1:18080",
				LoggingAvailable: true,
			},
			key:  "windows:healthy",
			icon: assets.TrayWindowsHealthyICO,
		},
		{
			name:   "degraded",
			status: app.Status{State: app.StateStarting},
			key:    "windows:degraded",
			icon:   assets.TrayWindowsDegradedICO,
		},
		{
			name:   "error",
			status: app.Status{State: app.StateListenerError},
			key:    "windows:error",
			icon:   assets.TrayWindowsErrorICO,
		},
		{
			name:   "stopped",
			status: app.Status{State: app.StateStopped},
			key:    "windows:stopped",
			icon:   assets.TrayWindowsStoppedICO,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := platformPresentationFor(test.status, "Kitsune Proxy")
			if got.iconKey != test.key || !bytes.Equal(got.icon, test.icon) {
				t.Fatalf("presentation = key %q icon-match %t", got.iconKey, bytes.Equal(got.icon, test.icon))
			}
			if got.template || got.title != "" || got.tooltip != "Kitsune Proxy" {
				t.Fatalf(
					"Windows metadata = template %t title %q tooltip %q",
					got.template,
					got.title,
					got.tooltip,
				)
			}
		})
	}
}
