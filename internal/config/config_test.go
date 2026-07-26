package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureCreatesLoadableTemplate(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	paths := Paths{
		BaseDir:    filepath.Join(base, ".kitsune"),
		ConfigFile: filepath.Join(base, ".kitsune", "config.yaml"),
		LogsDir:    filepath.Join(base, ".kitsune", "logs"),
	}

	created, err := Ensure(paths)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !created {
		t.Fatal("Ensure() created = false, want true")
	}

	cfg, err := Load(paths.ConfigFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != DefaultPort {
		t.Fatalf("port = %d, want %d", cfg.Server.Port, DefaultPort)
	}
	if !strings.HasPrefix(cfg.Server.APIKey, "kitsune-") {
		t.Fatalf("server API key has unexpected prefix")
	}
	if len(cfg.Server.APIKey) != len("kitsune-")+43 {
		t.Fatalf("server API key length = %d, want %d", len(cfg.Server.APIKey), len("kitsune-")+43)
	}
	if len(cfg.Upstreams) != 0 {
		t.Fatalf("upstreams = %d, want 0", len(cfg.Upstreams))
	}

	created, err = Ensure(paths)
	if err != nil {
		t.Fatalf("second Ensure() error = %v", err)
	}
	if created {
		t.Fatal("second Ensure() created = true, want false")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(paths.ConfigFile)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("config mode = %o, want 600", got)
		}

		if err := os.Chmod(paths.ConfigFile, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Ensure(paths); err != nil {
			t.Fatalf("Ensure() securing existing config error = %v", err)
		}
		info, err = os.Stat(paths.ConfigFile)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("secured existing config mode = %o, want 600", got)
		}
	}
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, `version: 1
server:
  api_key: kitsune-local
logging: {}
upstreams: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != DefaultPort || cfg.Logging.Level != DefaultLogLevel {
		t.Fatalf("defaults = port %d level %q", cfg.Server.Port, cfg.Logging.Level)
	}
}

func TestLoadRejectsDuplicateKeysAndMultipleDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "duplicate key",
			yaml: `version: 1
version: 1
server: {api_key: kitsune-local}
logging: {}
upstreams: {}
`,
		},
		{
			name: "multiple documents",
			yaml: `version: 1
server: {api_key: kitsune-local}
logging: {}
upstreams: {}
---
version: 1
`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(writeConfig(t, test.yaml)); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadValidConfiguration(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `version: 1
server:
  port: 19090
  api_key: kitsune-local
logging:
  level: debug
upstreams:
  openai:
    url: https://api.openai.com
    auth:
      mode: replace
      api_key: sk-upstream
    models:
      - id: gpt-5
        alias: openai-gpt-5
  local:
    url: http://127.0.0.1:11434
    auth:
      mode: none
    models:
      - id: llama3.3
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Upstreams["openai"].Models[0].PublicName() != "openai-gpt-5" {
		t.Fatalf("public model name not loaded")
	}
	if cfg.Upstreams["local"].Auth.APIKey != nil {
		t.Fatalf("none auth unexpectedly has an API key")
	}
}

func TestLoadAcceptsUpstreamBaseURLPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "OpenRouter", baseURL: "https://openrouter.ai/api"},
		{name: "Alibaba Model Studio", baseURL: "https://workspace.cn-beijing.maas.aliyuncs.com/apps/anthropic"},
		{name: "trailing slash", baseURL: "https://openrouter.ai/api/"},
	}
	template := `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  test-upstream:
    url: BASE_URL
    auth: {mode: none}
    models: [{id: model}]
`

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := Load(writeConfig(t, strings.Replace(template, "BASE_URL", test.baseURL, 1)))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := cfg.Upstreams["test-upstream"].URL; got != test.baseURL {
				t.Fatalf("upstream URL = %q", got)
			}
		})
	}
}

func TestLoadRejectsUnsafeUpstreamBaseURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "userinfo", baseURL: "https://user:password@example.com/api"},
		{name: "query", baseURL: "https://example.com/api?version=1"},
		{name: "fragment", baseURL: "https://example.com/api#section"},
		{name: "empty fragment", baseURL: "https://example.com/api#"},
		{name: "unsupported scheme", baseURL: "ftp://example.com/api"},
	}
	template := `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  invalid:
    url: BASE_URL
    auth: {mode: none}
    models: [{id: model}]
`

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, strings.Replace(template, "BASE_URL", test.baseURL, 1)))
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
		})
	}
}

func TestLoadRejectsInvalidConfigurationsWithoutExposingSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		yaml   string
		secret string
	}{
		{
			name: "unknown field",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info, api_key: should-not-leak}
upstreams: {}
`,
			secret: "should-not-leak",
		},
		{
			name: "replace key missing",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  openai:
    url: https://api.openai.com
    auth: {mode: replace}
    models: [{id: gpt-5}]
`,
		},
		{
			name: "none key present",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  local:
    url: http://127.0.0.1:11434
    auth: {mode: none, api_key: secret-local}
    models: [{id: llama3}]
`,
			secret: "secret-local",
		},
		{
			name: "none key explicitly null",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  local:
    url: http://127.0.0.1:11434
    auth: {mode: none, api_key: null}
    models: [{id: llama3}]
`,
		},
		{
			name: "numeric local key",
			yaml: `version: 1
server: {port: 18080, api_key: 12345}
logging: {level: info}
upstreams: {}
`,
		},
		{
			name: "null model alias",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  local:
    url: http://127.0.0.1:11434
    auth: {mode: none}
    models: [{id: llama3, alias: null}]
`,
		},
		{
			name: "duplicate public model",
			yaml: `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  first:
    url: https://first.example
    auth: {mode: none}
    models: [{id: shared}]
  second:
    url: https://second.example
    auth: {mode: none}
    models: [{id: shared}]
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(writeConfig(t, test.yaml))
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
			if test.secret != "" && strings.Contains(err.Error(), test.secret) {
				t.Fatalf("error exposed secret: %v", err)
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
