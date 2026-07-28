package router

import (
	"testing"

	"github.com/zhoux77899/kitsune-proxy/internal/config"
)

func TestTableResolvesAliasesExactly(t *testing.T) {
	t.Parallel()

	key := "upstream-key"
	table, err := New(config.Config{
		Upstreams: map[string]config.UpstreamConfig{
			"hosted": {
				URL:  "https://hosted.example",
				Auth: config.AuthConfig{Mode: "replace", APIKey: &key},
				Models: []config.ModelConfig{
					{ID: "chat-model-v2", Alias: "hosted-chat"},
					{ID: "text-embedding-model-v1"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	route, ok := table.Resolve("hosted-chat")
	if !ok {
		t.Fatal("Resolve(alias) ok = false")
	}
	if route.UpstreamModel != "chat-model-v2" || route.APIKey != key {
		t.Fatalf("route = %#v", route)
	}
	if _, ok := table.Resolve("chat-model-v2"); ok {
		t.Fatal("Resolve(upstream ID) ok = true when alias is configured")
	}
	if _, ok := table.Resolve("Hosted-Chat"); ok {
		t.Fatal("Resolve() unexpectedly ignored case")
	}

	models := table.Models()
	if len(models) != 2 || models[0].ID != "hosted-chat" {
		t.Fatalf("Models() = %#v", models)
	}
}

func TestTablePreservesUpstreamBaseURL(t *testing.T) {
	t.Parallel()

	table, err := New(config.Config{
		Upstreams: map[string]config.UpstreamConfig{
			"model-gateway": {
				URL:    "https://gateway.example/apps/messages",
				Auth:   config.AuthConfig{Mode: "none"},
				Models: []config.ModelConfig{{ID: "reasoning-model-v1"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	route, ok := table.Resolve("reasoning-model-v1")
	if !ok {
		t.Fatal("Resolve() ok = false")
	}
	if got := route.BaseURL.String(); got != "https://gateway.example/apps/messages" {
		t.Fatalf("upstream base URL = %q", got)
	}
}

func TestTablePreservesEscapedUpstreamBaseURLPath(t *testing.T) {
	t.Parallel()

	table, err := New(config.Config{
		Upstreams: map[string]config.UpstreamConfig{
			"escaped": {
				URL:    "https://upstream.example/base%2Fsegment",
				Auth:   config.AuthConfig{Mode: "none"},
				Models: []config.ModelConfig{{ID: "model"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	route, ok := table.Resolve("model")
	if !ok {
		t.Fatal("Resolve() ok = false")
	}
	if route.BaseURL.Path != "/base/segment" || route.BaseURL.RawPath != "/base%2Fsegment" {
		t.Fatalf("upstream base URL path = %q, raw path = %q", route.BaseURL.Path, route.BaseURL.RawPath)
	}
}

func TestTablePreservesPerUpstreamTLSVerificationPolicy(t *testing.T) {
	t.Parallel()

	table, err := New(config.Config{
		Upstreams: map[string]config.UpstreamConfig{
			"strict": {
				URL:    "https://strict.example",
				Auth:   config.AuthConfig{Mode: "none"},
				Models: []config.ModelConfig{{ID: "strict-model"}},
			},
			"internal": {
				URL:    "https://internal.example",
				TLS:    config.TLSConfig{SkipVerify: true},
				Auth:   config.AuthConfig{Mode: "none"},
				Models: []config.ModelConfig{{ID: "internal-model"}, {ID: "internal-model-2"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, model := range []string{"internal-model", "internal-model-2"} {
		route, ok := table.Resolve(model)
		if !ok || !route.SkipTLSVerify {
			t.Fatalf("Resolve(%q) = %#v, %t; want skip TLS verification", model, route, ok)
		}
	}
	strictRoute, ok := table.Resolve("strict-model")
	if !ok || strictRoute.SkipTLSVerify {
		t.Fatalf("Resolve(strict-model) = %#v, %t; want strict TLS verification", strictRoute, ok)
	}
}
