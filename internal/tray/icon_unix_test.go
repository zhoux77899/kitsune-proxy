//go:build !windows && !darwin

package tray

import (
	"bytes"
	"testing"

	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func TestUnixPresentationKeepsColorIconAndTitle(t *testing.T) {
	t.Parallel()

	got := platformPresentationFor(app.Status{State: app.StateRunning}, "Kitsune Proxy")
	if got.iconKey != "unix:color" || !bytes.Equal(got.icon, assets.TrayPNG) {
		t.Fatalf("Unix presentation = key %q icon-match %t", got.iconKey, bytes.Equal(got.icon, assets.TrayPNG))
	}
	if got.template || got.title != "Kitsune Proxy" || got.tooltip != "Kitsune Proxy" {
		t.Fatalf(
			"Unix metadata = template %t title %q tooltip %q",
			got.template,
			got.title,
			got.tooltip,
		)
	}
}
