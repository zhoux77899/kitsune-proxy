package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/config"
	"github.com/zhoux77899/kitsune-proxy/internal/router"
)

type testLocalizer struct{}

func (testLocalizer) Message(code string) string {
	return "message:" + code
}

func TestHandlerReplacesLocalBaseURLWithUpstreamBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		baseURL    string
		requestURL string
		want       string
	}{
		{
			name:       "OpenRouter",
			baseURL:    "https://openrouter.ai/api",
			requestURL: "http://127.0.0.1:18080/v1/messages?beta=true",
			want:       "https://openrouter.ai/api/v1/messages?beta=true",
		},
		{
			name:       "Alibaba Model Studio",
			baseURL:    "https://workspace.cn-beijing.maas.aliyuncs.com/apps/anthropic",
			requestURL: "http://127.0.0.1:18080/v1/messages",
			want:       "https://workspace.cn-beijing.maas.aliyuncs.com/apps/anthropic/v1/messages",
		},
		{
			name:       "trailing slash",
			baseURL:    "https://openrouter.ai/api/",
			requestURL: "http://127.0.0.1:18080/v1/messages",
			want:       "https://openrouter.ai/api/v1/messages",
		},
		{
			name:       "root base URL",
			baseURL:    "https://api.anthropic.com",
			requestURL: "http://127.0.0.1:18080/v1/messages",
			want:       "https://api.anthropic.com/v1/messages",
		},
		{
			name:       "escaped paths",
			baseURL:    "https://upstream.example/base%2Fsegment",
			requestURL: "http://127.0.0.1:18080/v1/files/a%2Fb?download=true",
			want:       "https://upstream.example/base%2Fsegment/v1/files/a%2Fb?download=true",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamURL string
			transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				upstreamURL = request.URL.String()
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Request:    request,
				}, nil
			})
			cfg := config.Config{Upstreams: map[string]config.UpstreamConfig{
				"test-upstream": {
					URL:    test.baseURL,
					Auth:   config.AuthConfig{Mode: "none"},
					Models: []config.ModelConfig{{ID: "test-model"}},
				},
			}}
			table, err := router.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			handler := New(
				&Runtime{LocalAPIKey: "local-key", Table: table},
				transport,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				testLocalizer{},
			)
			request := httptest.NewRequest(
				http.MethodPost,
				test.requestURL,
				strings.NewReader(`{"model":"test-model"}`),
			)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer local-key")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if upstreamURL != test.want {
				t.Fatalf("upstream URL = %q, want %q", upstreamURL, test.want)
			}
		})
	}
}

func TestHandlerRewritesAliasAndBearerKey(t *testing.T) {
	t.Parallel()

	type capturedRequest struct {
		method             string
		path               string
		query              string
		auth               string
		apiKey             string
		proxyAuthorization string
		forwardedFor       string
		contentLength      int64
		transferEncoding   []string
		body               string
	}
	captured := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		captured <- capturedRequest{
			method:             request.Method,
			path:               request.URL.Path,
			query:              request.URL.RawQuery,
			auth:               request.Header.Get("Authorization"),
			apiKey:             request.Header.Get("X-Api-Key"),
			proxyAuthorization: request.Header.Get("Proxy-Authorization"),
			forwardedFor:       request.Header.Get("X-Forwarded-For"),
			contentLength:      request.ContentLength,
			transferEncoding:   append([]string(nil), request.TransferEncoding...),
			body:               string(body),
		}
		writer.Header().Set("X-Upstream", "yes")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("upstream-response"))
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "replace", "upstream-key", config.ModelConfig{
		ID: "gpt-5", Alias: "provider-a-gpt-5",
	})
	body := "{\n  \"model\" : \"provider-a-gpt-5\",\n  \"input\": 1.0\n}"
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses?trace=no-log", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-key")
	request.Header.Set("Proxy-Authorization", "Basic must-not-pass")
	request.Header.Set("X-Forwarded-For", "must-not-pass")
	request.ContentLength = -1
	request.TransferEncoding = []string{"chunked"}
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Upstream") != "yes" || recorder.Body.String() != "upstream-response" {
		t.Fatalf("response was not passed through: %#v %q", recorder.Header(), recorder.Body.String())
	}
	got := <-captured
	if got.method != http.MethodPost || got.path != "/v1/responses" || got.query != "trace=no-log" {
		t.Fatalf("request target changed: %#v", got)
	}
	if got.auth != "Bearer upstream-key" || got.apiKey != "" {
		t.Fatalf("auth replacement = %#v", got)
	}
	if got.proxyAuthorization != "" || got.forwardedFor != "" {
		t.Fatalf("proxy-only headers reached upstream: %#v", got)
	}
	wantBody := "{\n  \"model\" : \"gpt-5\",\n  \"input\": 1.0\n}"
	if got.body != wantBody {
		t.Fatalf("body = %q, want %q", got.body, wantBody)
	}
	if got.contentLength != int64(len(wantBody)) {
		t.Fatalf("Content-Length = %d, want %d", got.contentLength, len(wantBody))
	}
	if len(got.transferEncoding) != 0 {
		t.Fatalf("Transfer-Encoding = %v, want none", got.transferEncoding)
	}
}

func TestHandlerPreservesUnaliasedBodyAndAPIKeyFormat(t *testing.T) {
	t.Parallel()

	var gotBody []byte
	var gotAuthorization string
	var gotAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotBody, _ = io.ReadAll(request.Body)
		gotAuthorization = request.Header.Get("Authorization")
		gotAPIKey = request.Header.Get("X-Api-Key")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "replace", "upstream-key", config.ModelConfig{ID: "same-model"})
	body := []byte(`{"keep":1.00, "model":"same-model"}`)
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Api-Key", "local-key")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("body changed: %q != %q", gotBody, body)
	}
	if gotAuthorization != "" || gotAPIKey != "upstream-key" {
		t.Fatalf("headers = Authorization %q, X-Api-Key %q", gotAuthorization, gotAPIKey)
	}
}

func TestHandlerAuthNoneRemovesInboundCredentials(t *testing.T) {
	t.Parallel()

	var gotAuthorization string
	var gotAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		gotAPIKey = request.Header.Get("X-Api-Key")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "none", "", config.ModelConfig{ID: "local-model"})
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", strings.NewReader(`{"model":"local-model"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-key")
	request.Header.Set("X-Api-Key", "local-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if gotAuthorization != "" || gotAPIKey != "" {
		t.Fatalf("auth headers reached no-auth upstream")
	}
}

func TestHandlerReplacesBothCredentialHeaders(t *testing.T) {
	t.Parallel()

	var gotAuthorization string
	var gotAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		gotAPIKey = request.Header.Get("X-Api-Key")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "replace", "upstream-key", config.ModelConfig{ID: "model"})
	request := httptest.NewRequest(
		http.MethodPost,
		"http://kitsune.local/v1/responses",
		strings.NewReader(`{"model":"model"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-key")
	request.Header.Set("X-Api-Key", "local-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if gotAuthorization != "Bearer upstream-key" || gotAPIKey != "upstream-key" {
		t.Fatalf("headers = Authorization %q, X-Api-Key %q", gotAuthorization, gotAPIKey)
	}
}

func TestHandlerRewritesGzipAlias(t *testing.T) {
	t.Parallel()

	var decodedUpstream []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		reader, err := gzip.NewReader(request.Body)
		if err != nil {
			t.Errorf("gzip.NewReader() error = %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		decodedUpstream, _ = io.ReadAll(reader)
		_ = reader.Close()
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "replace", "upstream-key", config.ModelConfig{
		ID: "real-model", Alias: "public-model",
	})
	compressed, err := gzipBody([]byte(`{"model":"public-model","keep":"yes"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", bytes.NewReader(compressed))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header.Set("Authorization", "Bearer local-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := string(decodedUpstream); got != `{"model":"real-model","keep":"yes"}` {
		t.Fatalf("decoded body = %q", got)
	}
}

func TestHandlerPreservesUnaliasedGzipBytes(t *testing.T) {
	t.Parallel()

	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamBody, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "none", "", config.ModelConfig{ID: "same-model"})
	compressed, err := gzipBody([]byte(`{"model":"same-model","keep":1.00}`))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"http://kitsune.local/v1/responses",
		bytes.NewReader(compressed),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header.Set("Authorization", "Bearer local-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Equal(upstreamBody, compressed) {
		t.Fatal("unaliased gzip body bytes changed")
	}
}

func TestHandlerLocalEndpointsAndAuthentication(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()
	handler := newTestHandler(t, upstream.URL, "none", "", config.ModelConfig{ID: "model-a", Alias: "public-a"})

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "http://kitsune.local/healthz", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"models":1`) {
		t.Fatalf("health response = %d %s", health.Code, health.Body.String())
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "http://kitsune.local/v1/models", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized models status = %d", unauthorized.Code)
	}

	modelRequest := httptest.NewRequest(http.MethodGet, "http://kitsune.local/v1/models", nil)
	modelRequest.Header.Set("Authorization", "Bearer local-key")
	models := httptest.NewRecorder()
	handler.ServeHTTP(models, modelRequest)
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"id":"public-a"`) ||
		strings.Contains(models.Body.String(), `"id":"model-a"`) {
		t.Fatalf("models response = %d %s", models.Code, models.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("local endpoints contacted upstream %d times", calls.Load())
	}
}

func TestHandlerRejectsBeforeUpstream(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()
	handler := newTestHandler(t, upstream.URL, "replace", "upstream-key", config.ModelConfig{
		ID: "real", Alias: "public",
	})

	tests := []struct {
		name   string
		auth   string
		body   string
		header http.Header
		status int
		code   string
	}{
		{name: "invalid auth", auth: "wrong", body: `{"model":"public"}`, status: 401, code: "invalid_api_key"},
		{name: "unknown model", auth: "local-key", body: `{"model":"unknown"}`, status: 404, code: "unknown_model"},
		{name: "duplicate model", auth: "local-key", body: `{"model":"public","model":"public"}`, status: 400, code: "duplicate_model"},
		{
			name: "integrity header with alias", auth: "local-key", body: `{"model":"public"}`,
			header: http.Header{"Content-Digest": {"sha-256=:abcd:"}},
			status: 400, code: "body_integrity_not_supported",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+test.auth)
			for key, values := range test.header {
				request.Header[key] = values
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"`+test.code+`"`) {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests contacted upstream %d times", calls.Load())
	}
}

func TestHandlerAuthenticatesBeforeReadingBusinessBody(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, "https://upstream.invalid", "none", "", config.ModelConfig{ID: "model"})
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", nil)
	request.Body = panicReader{}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer wrong")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandlerRejectsUnsupportedBodies(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, "https://upstream.invalid", "none", "", config.ModelConfig{ID: "model"})
	tests := []struct {
		name          string
		contentType   string
		encoding      string
		contentLength int64
		status        int
		code          string
	}{
		{
			name: "media type", contentType: "text/plain",
			status: http.StatusUnsupportedMediaType, code: "unsupported_media_type",
		},
		{
			name: "content encoding", contentType: "application/json", encoding: "br",
			status: http.StatusUnsupportedMediaType, code: "unsupported_content_encoding",
		},
		{
			name: "declared body too large", contentType: "application/json",
			contentLength: MaxRequestBodyBytes + 1,
			status:        http.StatusRequestEntityTooLarge, code: "request_body_too_large",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"http://kitsune.local/v1/responses",
				strings.NewReader(`{"model":"model"}`),
			)
			request.Header.Set("Authorization", "Bearer local-key")
			request.Header.Set("Content-Type", test.contentType)
			if test.encoding != "" {
				request.Header.Set("Content-Encoding", test.encoding)
			}
			if test.contentLength != 0 {
				request.ContentLength = test.contentLength
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestHandlerStreamsSSEImmediately(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: first\n\n"))
		writer.(http.Flusher).Flush()
		<-release
		_, _ = writer.Write([]byte("data: second\n\n"))
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, "none", "", config.ModelConfig{ID: "stream-model"})
	server := httptest.NewServer(handler)
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"model":"stream-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-key")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	lineResult := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(response.Body).ReadString('\n')
		lineResult <- line
	}()
	select {
	case line := <-lineResult:
		if line != "data: first\n" {
			t.Fatalf("first line = %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first SSE event was buffered")
	}
	close(release)
}

func TestHandlerPropagatesCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	cancelled := make(chan struct{})
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		close(cancelled)
		return nil, request.Context().Err()
	})
	cfg := config.Config{Upstreams: map[string]config.UpstreamConfig{
		"test-upstream": {
			URL:    "https://upstream.invalid",
			Auth:   config.AuthConfig{Mode: "none"},
			Models: []config.ModelConfig{{ID: "cancel-model"}},
		},
	}}
	table, err := router.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		&Runtime{LocalAPIKey: "local-key", Table: table},
		transport,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testLocalizer{},
	)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "http://kitsune.local/v1/responses", strings.NewReader(`{"model":"cancel-model"}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-key")
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream context was not cancelled")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy handler did not return after cancellation")
	}
}

func TestHandlerClassifiesUpstreamTransportErrors(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Upstreams: map[string]config.UpstreamConfig{
		"test-upstream": {
			URL:    "https://upstream.invalid",
			Auth:   config.AuthConfig{Mode: "none"},
			Models: []config.ModelConfig{{ID: "model"}},
		},
	}}
	table, err := router.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name: "connection", err: errors.New("connection failed"),
			status: http.StatusBadGateway, code: "upstream_error",
		},
		{
			name: "response header timeout", err: timeoutError{},
			status: http.StatusGatewayTimeout, code: "upstream_timeout",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			handler := New(
				&Runtime{LocalAPIKey: "local-key", Table: table},
				roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return nil, test.err
				}),
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				testLocalizer{},
			)
			request := httptest.NewRequest(
				http.MethodPost,
				"http://kitsune.local/v1/responses",
				strings.NewReader(`{"model":"model"}`),
			)
			request.Header.Set("Authorization", "Bearer local-key")
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestHandlerLogsNeverContainCredentialsHeadersBodiesOrQueries(t *testing.T) {
	t.Parallel()

	const (
		localKey     = "kitsune-super-secret-local"
		upstreamKey  = "sk-super-secret-upstream"
		bodySecret   = "private-prompt-value"
		querySecret  = "private-query-value"
		headerSecret = "private-header-value"
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	key := upstreamKey
	cfg := config.Config{Upstreams: map[string]config.UpstreamConfig{
		"safe-upstream": {
			URL:  upstream.URL,
			Auth: config.AuthConfig{Mode: "replace", APIKey: &key},
			Models: []config.ModelConfig{{
				ID: "real-model", Alias: "public-model",
			}},
		},
	}}
	table, err := router.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler := New(
		&Runtime{LocalAPIKey: localKey, Table: table},
		nil,
		slog.New(slog.NewTextHandler(&logs, nil)),
		testLocalizer{},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"http://kitsune.local/v1/responses?token="+querySecret,
		strings.NewReader(`{"model":"public-model","input":"`+bodySecret+`"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+localKey)
	request.Header.Set("X-Private", headerSecret)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	logText := logs.String()
	for _, forbidden := range []string{localKey, upstreamKey, bodySecret, querySecret, headerSecret} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("log contains forbidden value %q: %s", forbidden, logText)
		}
	}
	for _, required := range []string{
		"event=request_completed",
		"path=/v1/responses",
		"model=public-model",
		"upstream_model=real-model",
		"upstream=safe-upstream",
		"auth_method=bearer",
		"status=500",
	} {
		if !strings.Contains(logText, required) {
			t.Fatalf("log missing %q: %s", required, logText)
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("business body was read before authentication")
}

func (panicReader) Close() error {
	return nil
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "response header timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func newTestHandler(t *testing.T, origin, authMode, upstreamKey string, models ...config.ModelConfig) *Handler {
	t.Helper()
	var keyPointer *string
	if authMode == "replace" {
		key := upstreamKey
		keyPointer = &key
	}
	cfg := config.Config{
		Upstreams: map[string]config.UpstreamConfig{
			"test-upstream": {
				URL:    origin,
				Auth:   config.AuthConfig{Mode: authMode, APIKey: keyPointer},
				Models: models,
			},
		},
	}
	table, err := router.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(&Runtime{LocalAPIKey: "local-key", Table: table}, nil, logger, testLocalizer{})
}

func decodeJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
