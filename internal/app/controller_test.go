package app

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/config"
	"github.com/zhoux77899/kitsune-proxy/internal/i18n"
	"github.com/zhoux77899/kitsune-proxy/internal/logging"
)

func TestControllerStartsAndReloadsTransactionally(t *testing.T) {
	t.Parallel()

	upstreamOne := startUpstream(t, "first")
	upstreamTwo := startUpstream(t, "second")
	portOne := freePort(t)
	portTwo := freePort(t)

	base := t.TempDir()
	paths := config.Paths{
		BaseDir:    filepath.Join(base, ".kitsune"),
		ConfigFile: filepath.Join(base, ".kitsune", "config.yaml"),
		LogsDir:    filepath.Join(base, ".kitsune", "logs"),
	}
	if err := os.MkdirAll(paths.BaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRuntimeConfig(t, paths.ConfigFile, portOne, "local-one", upstreamOne, "public-one", "real-one")

	logs := logging.New(paths.LogsDir)
	t.Cleanup(func() { _ = logs.Close() })
	controller := New(paths, logs, i18n.New(i18n.English), Options{ShutdownTimeout: time.Second})
	controller.Start()
	t.Cleanup(controller.Shutdown)

	if status := controller.Status(); status.State != StateRunning || status.Models != 1 {
		t.Fatalf("initial status = %#v", status)
	}
	assertProxyResponse(t, portOne, "local-one", "public-one", "first:real-one")

	writeRuntimeConfig(t, paths.ConfigFile, portTwo, "local-two", upstreamTwo, "public-two", "real-two")
	if err := controller.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	assertProxyResponse(t, portTwo, "local-two", "public-two", "second:real-two")

	response, err := proxyRequest(portTwo, "local-one", "public-two")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old local key status = %d", response.StatusCode)
	}

	if err := os.WriteFile(paths.ConfigFile, []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := controller.Reload(); err == nil {
		t.Fatal("Reload() error = nil for invalid config")
	}
	if status := controller.Status(); status.State != StateConfigError ||
		status.Address != fmt.Sprintf("http://127.0.0.1:%d", portTwo) {
		t.Fatalf("failed reload status = %#v", status)
	}
	assertProxyResponse(t, portTwo, "local-two", "public-two", "second:real-two")

	writeRuntimeConfig(t, paths.ConfigFile, portTwo, "local-three", upstreamTwo, "public-three", "real-three")
	if err := controller.Reload(); err != nil {
		t.Fatalf("recovery Reload() error = %v", err)
	}
	if status := controller.Status(); status.State != StateRunning {
		t.Fatalf("recovered status = %#v", status)
	}
	assertProxyResponse(t, portTwo, "local-three", "public-three", "second:real-three")
}

func TestControllerInvalidStartupKeepsListenerStopped(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	base := t.TempDir()
	paths := config.Paths{
		BaseDir:    filepath.Join(base, ".kitsune"),
		ConfigFile: filepath.Join(base, ".kitsune", "config.yaml"),
		LogsDir:    filepath.Join(base, ".kitsune", "logs"),
	}
	if err := os.MkdirAll(paths.BaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfigFile, []byte(fmt.Sprintf(`version: 1
server: {port: %d, api_key: local}
logging: {level: invalid}
upstreams: {}
`, port)), 0o600); err != nil {
		t.Fatal(err)
	}

	logs := logging.New(paths.LogsDir)
	t.Cleanup(func() { _ = logs.Close() })
	controller := New(paths, logs, i18n.New(i18n.English), Options{})
	controller.Start()
	t.Cleanup(controller.Shutdown)

	if status := controller.Status(); status.State != StateConfigError {
		t.Fatalf("status = %#v", status)
	}
	connection, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("invalid configuration unexpectedly started listener")
	}
}

func startUpstream(t *testing.T, name string) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		model := extractModelForTest(string(body))
		_, _ = writer.Write([]byte(name + ":" + model))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
	})
	return "http://" + listener.Addr().String()
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func writeRuntimeConfig(t *testing.T, path string, port int, localKey, upstreamURL, publicModel, realModel string) {
	t.Helper()
	content := fmt.Sprintf(`version: 1
server:
  port: %d
  api_key: %s
logging:
  level: info
upstreams:
  test:
    url: %s
    auth:
      mode: none
    models:
      - id: %s
        alias: %s
`, port, localKey, upstreamURL, realModel, publicModel)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertProxyResponse(t *testing.T, port int, key, model, want string) {
	t.Helper()
	response, err := proxyRequest(port, key, model)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) != want {
		t.Fatalf("response = %d %q, want %q", response.StatusCode, body, want)
	}
}

func proxyRequest(port int, key, model string) (*http.Response, error) {
	request, _ := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/responses", port),
		strings.NewReader(fmt.Sprintf(`{"model":%q}`, model)),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 2 * time.Second}
	return client.Do(request)
}

func extractModelForTest(body string) string {
	const prefix = `"model":"`
	start := strings.Index(body, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
