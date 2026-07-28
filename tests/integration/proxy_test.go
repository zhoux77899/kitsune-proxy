package integration

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/app"
	"github.com/zhoux77899/kitsune-proxy/internal/config"
	"github.com/zhoux77899/kitsune-proxy/internal/i18n"
	"github.com/zhoux77899/kitsune-proxy/internal/logging"
)

func TestLoopbackApplicationRoutesAndReloadsCredentials(t *testing.T) {
	t.Parallel()

	type upstreamRequest struct {
		path          string
		query         string
		authorization string
		body          string
	}
	captured := make(chan upstreamRequest, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		captured <- upstreamRequest{
			path:          request.URL.Path,
			query:         request.URL.RawQuery,
			authorization: request.Header.Get("Authorization"),
			body:          string(body),
		}
		writer.Header().Set("X-Upstream", "integration")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("forwarded"))
	}))
	defer upstream.Close()

	base := t.TempDir()
	paths := config.Paths{
		BaseDir:    filepath.Join(base, ".kitsune"),
		ConfigFile: filepath.Join(base, ".kitsune", "config.yaml"),
		LogsDir:    filepath.Join(base, ".kitsune", "logs"),
	}
	if err := os.MkdirAll(paths.BaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	port := availablePort(t)
	upstreamBaseURL := upstream.URL + "/provider-base"
	writeConfig(t, paths.ConfigFile, port, upstreamBaseURL, "local-one", "upstream-one")

	logs := logging.New(paths.LogsDir)
	controller := app.New(paths, logs, i18n.New(i18n.English), app.Options{
		ShutdownTimeout: time.Second,
	})
	controller.Start()
	t.Cleanup(func() {
		controller.Shutdown()
		_ = logs.Close()
	})

	send := func(localKey string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(
			http.MethodPost,
			fmt.Sprintf("http://127.0.0.1:%d/v1/responses?preserve=yes", port),
			strings.NewReader("{\n  \"model\": \"public-model\",\n  \"input\": 1.0\n}"),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+localKey)
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	response := send("local-one")
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated ||
		response.Header.Get("X-Upstream") != "integration" ||
		string(responseBody) != "forwarded" {
		t.Fatalf("response = %d %#v %q", response.StatusCode, response.Header, responseBody)
	}
	first := <-captured
	if first.path != "/provider-base/v1/responses" || first.query != "preserve=yes" {
		t.Fatalf("upstream target = %#v", first)
	}
	if first.authorization != "Bearer upstream-one" {
		t.Fatalf("upstream authorization = %q", first.authorization)
	}
	if want := "{\n  \"model\": \"real-model\",\n  \"input\": 1.0\n}"; first.body != want {
		t.Fatalf("upstream body = %q, want %q", first.body, want)
	}

	writeConfig(t, paths.ConfigFile, port, upstreamBaseURL, "local-two", "upstream-two")
	if err := controller.Reload(); err != nil {
		t.Fatal(err)
	}
	oldKeyResponse := send("local-one")
	_ = oldKeyResponse.Body.Close()
	if oldKeyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old key status = %d", oldKeyResponse.StatusCode)
	}
	newKeyResponse := send("local-two")
	_ = newKeyResponse.Body.Close()
	if newKeyResponse.StatusCode != http.StatusCreated {
		t.Fatalf("new key status = %d", newKeyResponse.StatusCode)
	}
	second := <-captured
	if second.path != "/provider-base/v1/responses" || second.query != "preserve=yes" {
		t.Fatalf("reloaded upstream target = %#v", second)
	}
	if second.authorization != "Bearer upstream-two" {
		t.Fatalf("reloaded upstream authorization = %q", second.authorization)
	}
}

func TestLoopbackApplicationReloadsTLSVerificationPolicy(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte("trusted internal response"))
	}))
	upstream.Config.ErrorLog = log.New(io.Discard, "", 0)
	upstream.StartTLS()
	defer upstream.Close()

	base := t.TempDir()
	paths := config.Paths{
		BaseDir:    filepath.Join(base, ".kitsune"),
		ConfigFile: filepath.Join(base, ".kitsune", "config.yaml"),
		LogsDir:    filepath.Join(base, ".kitsune", "logs"),
	}
	if err := os.MkdirAll(paths.BaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	port := availablePort(t)
	writeTLSConfig(t, paths.ConfigFile, port, upstream.URL, false)

	logs := logging.New(paths.LogsDir)
	controller := app.New(paths, logs, i18n.New(i18n.English), app.Options{
		ShutdownTimeout: time.Second,
	})
	controller.Start()
	t.Cleanup(func() {
		controller.Shutdown()
		_ = logs.Close()
	})

	send := func() (int, string) {
		t.Helper()
		request, err := http.NewRequest(
			http.MethodPost,
			fmt.Sprintf("http://127.0.0.1:%d/v1/responses", port),
			strings.NewReader(`{"model":"internal-model"}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer local-key")
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}

	status, body := send()
	if status != http.StatusBadGateway || !strings.Contains(body, "upstream_error") {
		t.Fatalf("strict response = %d %q, want 502 upstream_error", status, body)
	}

	writeTLSConfig(t, paths.ConfigFile, port, upstream.URL, true)
	if err := controller.Reload(); err != nil {
		t.Fatal(err)
	}
	status, body = send()
	if status != http.StatusAccepted || body != "trusted internal response" {
		t.Fatalf("skip-verify response = %d %q", status, body)
	}

	writeTLSConfig(t, paths.ConfigFile, port, upstream.URL, false)
	if err := controller.Reload(); err != nil {
		t.Fatal(err)
	}
	status, body = send()
	if status != http.StatusBadGateway || !strings.Contains(body, "upstream_error") {
		t.Fatalf("restored strict response = %d %q, want 502 upstream_error", status, body)
	}
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func writeConfig(t *testing.T, path string, port int, upstreamURL, localKey, upstreamKey string) {
	t.Helper()
	content := fmt.Sprintf(`version: 1
server:
  port: %d
  api_key: %s
logging:
  level: info
upstreams:
  integration:
    url: %s
    auth:
      mode: replace
      api_key: %s
    models:
      - id: real-model
        alias: public-model
`, port, localKey, upstreamURL, upstreamKey)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTLSConfig(t *testing.T, path string, port int, upstreamURL string, skipVerify bool) {
	t.Helper()
	content := fmt.Sprintf(`version: 1
server:
  port: %d
  api_key: local-key
logging:
  level: info
upstreams:
  internal:
    url: %s
    tls:
      skip_verify: %t
    auth:
      mode: none
    models:
      - id: internal-model
`, port, upstreamURL, skipVerify)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
