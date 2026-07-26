package tray

import (
	"testing"

	"github.com/zhoux77899/kitsune-proxy/internal/app"
	"github.com/zhoux77899/kitsune-proxy/internal/i18n"
)

func TestStatusTitleUsesSafeLocalizedState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status app.Status
		want   string
	}{
		{
			name:   "starting",
			status: app.Status{State: app.StateStarting},
			want:   "Starting",
		},
		{
			name:   "running",
			status: app.Status{State: app.StateRunning, Address: "http://127.0.0.1:18080"},
			want:   "Running on http://127.0.0.1:18080",
		},
		{
			name: "configuration error omits detail",
			status: app.Status{
				State:   app.StateConfigError,
				Address: "http://127.0.0.1:18080",
				Detail:  "server.api_key: secret",
			},
			want: "Configuration error; still running on http://127.0.0.1:18080",
		},
	}

	catalog := i18n.New(i18n.English)
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := statusTitle(catalog, test.status); got != test.want {
				t.Fatalf("statusTitle() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTrayHealthUsesServiceAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status app.Status
		want   trayHealth
	}{
		{
			name:   "starting is degraded",
			status: app.Status{State: app.StateStarting},
			want:   trayDegraded,
		},
		{
			name: "running with logging is healthy",
			status: app.Status{
				State:            app.StateRunning,
				Address:          "http://127.0.0.1:18080",
				LoggingAvailable: true,
			},
			want: trayHealthy,
		},
		{
			name: "running without logging is degraded",
			status: app.Status{
				State:   app.StateRunning,
				Address: "http://127.0.0.1:18080",
			},
			want: trayDegraded,
		},
		{
			name: "running without an address is invalid",
			status: app.Status{
				State:            app.StateRunning,
				LoggingAvailable: true,
			},
			want: trayError,
		},
		{
			name: "configuration error with active listener is degraded",
			status: app.Status{
				State:            app.StateConfigError,
				Address:          "http://127.0.0.1:18080",
				LoggingAvailable: true,
			},
			want: trayDegraded,
		},
		{
			name:   "configuration error without listener is error",
			status: app.Status{State: app.StateConfigError},
			want:   trayError,
		},
		{
			name: "listener error is error even with address",
			status: app.Status{
				State:   app.StateListenerError,
				Address: "http://127.0.0.1:18080",
			},
			want: trayError,
		},
		{
			name:   "stopped is stopped",
			status: app.Status{State: app.StateStopped},
			want:   trayStopped,
		},
		{
			name:   "unknown state fails closed",
			status: app.Status{State: app.State("future")},
			want:   trayError,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := trayHealthForStatus(test.status); got != test.want {
				t.Fatalf("trayHealthForStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
