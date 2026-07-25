//go:build darwin

package autostart

import (
	"strings"
	"testing"
)

func TestLaunchAgentEscapesExecutable(t *testing.T) {
	t.Parallel()

	content := string(launchAgent(`/Applications/Kitsune & Fox.app/Contents/MacOS/kitsune-proxy`))
	if !strings.Contains(content, "Kitsune &amp; Fox.app") {
		t.Fatalf("launch agent did not XML-escape executable: %s", content)
	}
}
