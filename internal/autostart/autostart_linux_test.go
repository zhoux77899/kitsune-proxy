//go:build linux

package autostart

import "testing"

func TestQuoteDesktopExecutable(t *testing.T) {
	t.Parallel()

	got := quoteDesktopExecutable(`/opt/Kitsune Proxy/$current/%name`)
	if want := `"/opt/Kitsune Proxy/\$current/%%name"`; got != want {
		t.Fatalf("quoteDesktopExecutable() = %q, want %q", got, want)
	}
}
