package integration

import (
	"fmt"
	"io"
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
	writeConfig(t, paths.ConfigFile, port, upstream.URL, "local-one", "upstream-one")

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
	if first.path != "/v1/responses" || first.query != "preserve=yes" {
		t.Fatalf("upstream target = %#v", first)
	}
	if first.authorization != "Bearer upstream-one" {
		t.Fatalf("upstream authorization = %q", first.authorization)
	}
	if want := "{\n  \"model\": \"real-model\",\n  \"input\": 1.0\n}"; first.body != want {
		t.Fatalf("upstream body = %q, want %q", first.body, want)
	}

	writeConfig(t, paths.ConfigFile, port, upstream.URL, "local-two", "upstream-two")
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
	if second.authorization != "Bearer upstream-two" {
		t.Fatalf("reloaded upstream authorization = %q", second.authorization)
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
