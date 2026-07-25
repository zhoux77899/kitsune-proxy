package auth

import (
	"net/http"
	"testing"
)

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header http.Header
		want   Method
		ok     bool
	}{
		{name: "bearer", header: http.Header{"Authorization": {"Bearer local-key"}}, want: MethodBearer, ok: true},
		{name: "bearer case insensitive", header: http.Header{"Authorization": {"bearer local-key"}}, want: MethodBearer, ok: true},
		{name: "api key", header: http.Header{"X-Api-Key": {"local-key"}}, want: MethodAPIKey, ok: true},
		{name: "both", header: http.Header{"Authorization": {"Bearer local-key"}, "X-Api-Key": {"local-key"}}, want: MethodBoth, ok: true},
		{name: "missing", header: http.Header{}, ok: false},
		{name: "wrong bearer", header: http.Header{"Authorization": {"Bearer wrong"}}, ok: false},
		{name: "unsupported scheme", header: http.Header{"Authorization": {"Basic local-key"}}, ok: false},
		{name: "one of both wrong", header: http.Header{"Authorization": {"Bearer local-key"}, "X-Api-Key": {"wrong"}}, ok: false},
		{name: "duplicate header", header: http.Header{"X-Api-Key": {"local-key", "local-key"}}, ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Authenticate(test.header, "local-key")
			if (err == nil) != test.ok {
				t.Fatalf("Authenticate() error = %v, ok = %v", err, test.ok)
			}
			if got != test.want {
				t.Fatalf("Authenticate() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	t.Run("replace both", func(t *testing.T) {
		header := http.Header{
			"Authorization": {"Bearer local"},
			"X-Api-Key":     {"local"},
		}
		Apply(header, MethodBoth, "replace", "upstream")
		if got := header.Get("Authorization"); got != "Bearer upstream" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := header.Get("X-Api-Key"); got != "upstream" {
			t.Fatalf("X-Api-Key = %q", got)
		}
	})

	t.Run("none removes both", func(t *testing.T) {
		header := http.Header{
			"Authorization": {"Bearer local"},
			"X-Api-Key":     {"local"},
		}
		Apply(header, MethodBoth, "none", "")
		if header.Get("Authorization") != "" || header.Get("X-Api-Key") != "" {
			t.Fatalf("authentication headers were not removed: %#v", header)
		}
	})
}

func TestConstantTimeEqualHandlesDifferentLengths(t *testing.T) {
	t.Parallel()

	if !constantTimeEqual("same", "same") {
		t.Fatal("equal values did not match")
	}
	if constantTimeEqual("short", "a-much-longer-secret") {
		t.Fatal("different values matched")
	}
}
