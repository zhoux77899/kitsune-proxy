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
