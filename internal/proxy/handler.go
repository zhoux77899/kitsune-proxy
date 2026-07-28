// Package proxy implements local authentication, model routing, and forwarding.
package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/auth"
	"github.com/zhoux77899/kitsune-proxy/internal/logging"
	"github.com/zhoux77899/kitsune-proxy/internal/router"
)

const MaxRequestBodyBytes = int64(64 << 20)

// Localizer supplies human-readable messages for stable local error codes.
type Localizer interface {
	Message(code string) string
}

// Runtime is the immutable per-request authentication and routing snapshot.
type Runtime struct {
	LocalAPIKey string
	Table       *router.Table
}

// Handler is a concurrency-safe local reverse proxy.
type Handler struct {
	runtime      atomic.Pointer[Runtime]
	logger       *slog.Logger
	localizer    Localizer
	reverseProxy *httputil.ReverseProxy
	requestSeq   atomic.Uint64
}

type routeContextKey struct{}

type routeTransport struct {
	verified http.RoundTripper
	insecure http.RoundTripper
}

func newRouteTransport(base http.RoundTripper) *routeTransport {
	if base == nil {
		base = DefaultTransport()
	}

	insecure := base
	if transport, ok := base.(*http.Transport); ok {
		insecureTransport := transport.Clone()
		tlsConfig := insecureTransport.TLSClientConfig.Clone()
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		}
		// This transport is selected only for an upstream that explicitly opts out.
		tlsConfig.InsecureSkipVerify = true
		insecureTransport.TLSClientConfig = tlsConfig
		insecure = insecureTransport
	}

	return &routeTransport{verified: base, insecure: insecure}
}

func (t *routeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	route, ok := request.Context().Value(routeContextKey{}).(router.Route)
	if ok && route.SkipTLSVerify {
		return t.insecure.RoundTrip(request)
	}
	return t.verified.RoundTrip(request)
}

// New creates a handler. A nil runtime keeps the handler unavailable until Update.
func New(runtime *Runtime, transport http.RoundTripper, logger *slog.Logger, localizer Localizer) *Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	handler := &Handler{logger: logger, localizer: localizer}
	if runtime != nil {
		handler.runtime.Store(runtime)
	}
	transport = newRouteTransport(transport)

	handler.reverseProxy = &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			route := request.In.Context().Value(routeContextKey{}).(router.Route)
			request.SetURL(route.BaseURL)
			request.Out.RequestURI = ""
			request.Out.Header.Del("Forwarded")
			request.Out.Header.Del("X-Forwarded-For")
			request.Out.Header.Del("X-Forwarded-Host")
			request.Out.Header.Del("X-Forwarded-Proto")
		},
		Transport:     transport,
		FlushInterval: -1,
		ErrorLog:      log.New(io.Discard, "", 0),
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			if isTimeout(err) {
				handler.writeError(writer, http.StatusGatewayTimeout, "upstream_timeout")
				return
			}
			handler.writeError(writer, http.StatusBadGateway, "upstream_error")
		},
	}
	return handler
}

// DefaultTransport returns the production transport with response transparency.
func DefaultTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 5 * time.Minute,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true,
	}
}

// Update atomically changes the runtime used by newly arriving requests.
func (h *Handler) Update(runtime *Runtime) {
	h.runtime.Store(runtime)
}

// ServeHTTP authenticates, resolves, minimally rewrites, and forwards a request.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	tracker := &trackingWriter{ResponseWriter: writer}
	started := time.Now()
	requestID := h.requestID()
	authMethod := "none"
	publicModel := ""
	upstreamModel := ""
	upstreamName := ""
	requestBytes := int64(0)
	errorCode := ""

	defer func() {
		status := tracker.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		attrs := []any{
			"request_id", requestID,
			"method", request.Method,
			"path", request.URL.Path,
			"model", publicModel,
			"upstream_model", upstreamModel,
			"upstream", upstreamName,
			"auth_method", authMethod,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"request_bytes", requestBytes,
			"response_bytes", tracker.BytesWritten(),
		}
		if errorCode != "" {
			attrs = append(attrs, "error_code", errorCode)
		}
		logging.Event(context.Background(), h.logger, level, "request_completed", attrs...)
	}()

	if request.URL.Path == "/healthz" {
		if request.Method != http.MethodGet {
			tracker.Header().Set("Allow", http.MethodGet)
			errorCode = "method_not_allowed"
			h.writeError(tracker, http.StatusMethodNotAllowed, errorCode)
			return
		}
		runtime := h.runtime.Load()
		if runtime == nil {
			errorCode = "service_unavailable"
			h.writeError(tracker, http.StatusServiceUnavailable, errorCode)
			return
		}
		h.writeJSON(tracker, http.StatusOK, map[string]any{"status": "ok", "models": runtime.Table.Len()})
		return
	}

	runtime := h.runtime.Load()
	if runtime == nil {
		errorCode = "service_unavailable"
		h.writeError(tracker, http.StatusServiceUnavailable, errorCode)
		return
	}

	method, err := auth.Authenticate(request.Header, runtime.LocalAPIKey)
	if err != nil {
		errorCode = "invalid_api_key"
		h.writeError(tracker, http.StatusUnauthorized, errorCode)
		return
	}
	authMethod = string(method)

	if request.URL.Path == "/v1/models" {
		if request.Method != http.MethodGet {
			tracker.Header().Set("Allow", http.MethodGet)
			errorCode = "method_not_allowed"
			h.writeError(tracker, http.StatusMethodNotAllowed, errorCode)
			return
		}
		h.writeModels(tracker, runtime.Table.Models())
		return
	}

	if isWebSocketRequest(request) {
		errorCode = "websocket_not_supported"
		h.writeError(tracker, http.StatusBadRequest, errorCode)
		return
	}
	if !isJSONMediaType(request.Header.Get("Content-Type")) {
		errorCode = "unsupported_media_type"
		h.writeError(tracker, http.StatusUnsupportedMediaType, errorCode)
		return
	}

	body, err := readRequestBody(request)
	if err != nil {
		var localErr *requestError
		if errors.As(err, &localErr) {
			errorCode = localErr.code
			h.writeError(tracker, localErr.status, localErr.code)
			return
		}
		errorCode = "invalid_json"
		h.writeError(tracker, http.StatusBadRequest, errorCode)
		return
	}
	requestBytes = int64(len(body.raw))

	location, err := locateTopLevelModel(body.decoded)
	if err != nil {
		errorCode = modelErrorCode(err)
		h.writeError(tracker, http.StatusBadRequest, errorCode)
		return
	}
	publicModel = location.value

	route, ok := runtime.Table.Resolve(publicModel)
	if !ok {
		errorCode = "unknown_model"
		h.writeError(tracker, http.StatusNotFound, errorCode)
		return
	}
	upstreamModel = route.UpstreamModel
	upstreamName = route.UpstreamName

	forwardBody := body.raw
	if publicModel != route.UpstreamModel {
		if hasIntegrityHeader(request.Header) {
			errorCode = "body_integrity_not_supported"
			h.writeError(tracker, http.StatusBadRequest, errorCode)
			return
		}
		rewritten, err := replaceModel(body.decoded, location, route.UpstreamModel)
		if err != nil {
			errorCode = "invalid_model"
			h.writeError(tracker, http.StatusBadRequest, errorCode)
			return
		}
		if body.encoding == "gzip" {
			forwardBody, err = gzipBody(rewritten)
			if err != nil {
				errorCode = "invalid_json"
				h.writeError(tracker, http.StatusBadRequest, errorCode)
				return
			}
		} else {
			forwardBody = rewritten
		}
	}

	request.Body = io.NopCloser(bytes.NewReader(forwardBody))
	request.ContentLength = int64(len(forwardBody))
	request.TransferEncoding = nil
	request.Header.Del("Content-Length")
	auth.Apply(request.Header, method, route.AuthMode, route.APIKey)

	ctx := context.WithValue(request.Context(), routeContextKey{}, route)
	h.reverseProxy.ServeHTTP(tracker, request.WithContext(ctx))
}

type bufferedBody struct {
	raw      []byte
	decoded  []byte
	encoding string
}

func readRequestBody(request *http.Request) (bufferedBody, error) {
	if request.ContentLength > MaxRequestBodyBytes {
		return bufferedBody{}, &requestError{status: http.StatusRequestEntityTooLarge, code: "request_body_too_large"}
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, MaxRequestBodyBytes+1))
	if err != nil {
		return bufferedBody{}, &requestError{status: http.StatusBadRequest, code: "invalid_json"}
	}
	if int64(len(raw)) > MaxRequestBodyBytes {
		return bufferedBody{}, &requestError{status: http.StatusRequestEntityTooLarge, code: "request_body_too_large"}
	}

	encoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
		return bufferedBody{raw: raw, decoded: raw, encoding: "identity"}, nil
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return bufferedBody{}, &requestError{status: http.StatusBadRequest, code: "invalid_json"}
		}
		decoded, readErr := io.ReadAll(io.LimitReader(reader, MaxRequestBodyBytes+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			return bufferedBody{}, &requestError{status: http.StatusBadRequest, code: "invalid_json"}
		}
		if int64(len(decoded)) > MaxRequestBodyBytes {
			return bufferedBody{}, &requestError{status: http.StatusRequestEntityTooLarge, code: "request_body_too_large"}
		}
		return bufferedBody{raw: raw, decoded: decoded, encoding: "gzip"}, nil
	default:
		return bufferedBody{}, &requestError{status: http.StatusUnsupportedMediaType, code: "unsupported_content_encoding"}
	}
}

func gzipBody(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (h *Handler) writeModels(writer http.ResponseWriter, models []router.ModelInfo) {
	type modelObject struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	data := make([]modelObject, 0, len(models))
	for _, model := range models {
		data = append(data, modelObject{
			ID: model.ID, Object: "model", Created: 0, OwnedBy: model.Owner,
		})
	}
	h.writeJSON(writer, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (h *Handler) writeError(writer http.ResponseWriter, status int, code string) {
	h.writeJSON(writer, status, map[string]any{
		"error": map[string]any{
			"type":    "kitsune_proxy_error",
			"code":    code,
			"message": h.message(code),
		},
	})
}

func (h *Handler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func (h *Handler) message(code string) string {
	if h.localizer != nil {
		return h.localizer.Message(code)
	}
	return code
}

func (h *Handler) requestID() string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err == nil {
		return hex.EncodeToString(random)
	}
	return "fallback-" + strconv.FormatUint(h.requestSeq.Add(1), 10)
}

func isJSONMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func isWebSocketRequest(request *http.Request) bool {
	return strings.EqualFold(request.Header.Get("Upgrade"), "websocket") ||
		headerContainsToken(request.Header, "Connection", "upgrade")
}

func headerContainsToken(header http.Header, name, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func hasIntegrityHeader(header http.Header) bool {
	return header.Get("Content-MD5") != "" ||
		header.Get("Digest") != "" ||
		header.Get("Content-Digest") != ""
}

func modelErrorCode(err error) string {
	switch {
	case errors.Is(err, errMissingModel):
		return "missing_model"
	case errors.Is(err, errInvalidModel):
		return "invalid_model"
	case errors.Is(err, errDuplicateModel):
		return "duplicate_model"
	default:
		return "invalid_json"
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type requestError struct {
	status int
	code   string
}

func (e *requestError) Error() string {
	return fmt.Sprintf("%s (%d)", e.code, e.status)
}
