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
  hosted:
    url: https://hosted.example
    auth:
      mode: replace
      api_key: sk-upstream
    models:
      - id: chat-model-v2
        alias: hosted-chat
  local:
    url: http://127.0.0.1:11434
    auth:
      mode: none
    models:
      - id: local-model-v1
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Upstreams["hosted"].Models[0].PublicName() != "hosted-chat" {
		t.Fatalf("public model name not loaded")
	}
	if cfg.Upstreams["local"].Auth.APIKey != nil {
		t.Fatalf("none auth unexpectedly has an API key")
	}
}

func TestLoadTLSVerificationPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tls        string
		skipVerify bool
	}{
		{name: "omitted"},
		{name: "empty", tls: "    tls: {}\n"},
		{name: "explicit false", tls: "    tls: {skip_verify: false}\n"},
		{name: "explicit true", tls: "    tls: {skip_verify: true}\n", skipVerify: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(writeConfig(t, `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  internal:
    url: https://internal.example
`+test.tls+`    auth: {mode: none}
    models: [{id: model}]
`))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := cfg.Upstreams["internal"].TLS.SkipVerify; got != test.skipVerify {
				t.Fatalf("skip verify = %t, want %t", got, test.skipVerify)
			}
		})
	}
}

func TestLoadRejectsInvalidTLSVerificationPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		tls      string
		wantPath string
	}{
		{name: "non-mapping", url: "https://internal.example", tls: "true", wantPath: "upstreams.internal.tls"},
		{name: "null block", url: "https://internal.example", tls: "null", wantPath: "upstreams.internal.tls"},
		{name: "string value", url: "https://internal.example", tls: `{skip_verify: "true"}`, wantPath: "upstreams.internal.tls.skip_verify"},
		{name: "null value", url: "https://internal.example", tls: "{skip_verify: null}", wantPath: "upstreams.internal.tls.skip_verify"},
		{name: "duplicate field", url: "https://internal.example", tls: "{skip_verify: true, skip_verify: false}", wantPath: "upstreams.internal.tls.skip_verify"},
		{name: "unknown field", url: "https://internal.example", tls: "{skip_verify: true, server_name: internal.example}", wantPath: "upstreams.internal.tls.server_name"},
		{name: "HTTP upstream", url: "http://127.0.0.1:8443", tls: "{skip_verify: true}", wantPath: "upstreams.internal.tls.skip_verify"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(writeConfig(t, `version: 1
server: {port: 18080, api_key: kitsune-local}
logging: {level: info}
upstreams:
  internal:
    url: `+test.url+`
    tls: `+test.tls+`
    auth: {mode: none}
    models: [{id: model}]
`))
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
			validationError, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("Load() error = %T, want *ValidationError", err)
			}
			if validationError.Path != test.wantPath {
				t.Fatalf("validation path = %q, want %q", validationError.Path, test.wantPath)
			}
		})
	}
}

func TestLoadAcceptsUpstreamBaseURLPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "single base path", baseURL: "https://upstream.example/api"},
		{name: "nested base path", baseURL: "https://gateway.example/apps/messages"},
		{name: "trailing slash", baseURL: "https://upstream.example/api/"},
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
  hosted:
    url: https://hosted.example
    auth: {mode: replace}
    models: [{id: hosted-model}]
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
    models: [{id: local-model}]
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
    models: [{id: local-model}]
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
    models: [{id: local-model, alias: null}]
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
