//go:build darwin

package tray

import (
	"bytes"
	"testing"

	"github.com/zhoux77899/kitsune-proxy/assets"
	"github.com/zhoux77899/kitsune-proxy/internal/app"
)

func TestMacOSPresentationUsesTemplateWithoutText(t *testing.T) {
	t.Parallel()

	got := platformPresentationFor(app.Status{State: app.StateRunning}, "Kitsune Proxy")
	if got.iconKey != "macos:template" || !bytes.Equal(got.icon, assets.TrayMacOSTemplatePNG) {
		t.Fatalf("macOS presentation = key %q icon-match %t", got.iconKey, bytes.Equal(got.icon, assets.TrayMacOSTemplatePNG))
	}
	if !got.template || got.title != "" || got.tooltip != "" {
		t.Fatalf(
			"macOS metadata = template %t title %q tooltip %q",
			got.template,
			got.title,
			got.tooltip,
		)
	}
}
