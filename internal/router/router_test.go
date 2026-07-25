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
			"openai": {
				URL:  "https://api.openai.com",
				Auth: config.AuthConfig{Mode: "replace", APIKey: &key},
				Models: []config.ModelConfig{
					{ID: "gpt-5", Alias: "openai-gpt-5"},
					{ID: "text-embedding-3-small"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	route, ok := table.Resolve("openai-gpt-5")
	if !ok {
		t.Fatal("Resolve(alias) ok = false")
	}
	if route.UpstreamModel != "gpt-5" || route.APIKey != key {
		t.Fatalf("route = %#v", route)
	}
	if _, ok := table.Resolve("gpt-5"); ok {
		t.Fatal("Resolve(upstream ID) ok = true when alias is configured")
	}
	if _, ok := table.Resolve("OpenAI-GPT-5"); ok {
		t.Fatal("Resolve() unexpectedly ignored case")
	}

	models := table.Models()
	if len(models) != 2 || models[0].ID != "openai-gpt-5" {
		t.Fatalf("Models() = %#v", models)
	}
}
