//go:build windows

package autostart

import "testing"

func TestQuoteWindowsExecutable(t *testing.T) {
	t.Parallel()

	got, err := quoteWindowsExecutable(`C:\Program Files\Kitsune\kitsune-proxy.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"C:\Program Files\Kitsune\kitsune-proxy.exe"`; got != want {
		t.Fatalf("quoteWindowsExecutable() = %q, want %q", got, want)
	}
}
