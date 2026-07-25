// Package logging provides fixed-path structured logs with bounded rotation.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultMaxBytes = int64(10 << 20)
	DefaultBackups  = 5
	LogFileName     = "kitsune.log"
)

// Manager owns the logger, dynamic level, and rotating writer.
type Manager struct {
	logger    *slog.Logger
	level     slog.LevelVar
	writer    io.Closer
	available atomic.Bool
	errorMu   sync.Mutex
	initErr   error
	sinkMu    sync.Mutex
	sink      func(bool)
}

// New creates a file-backed logger or a stderr fallback when disk logging fails.
func New(logsDir string) *Manager {
	manager := &Manager{}
	manager.level.Set(slog.LevelInfo)

	var output io.Writer = os.Stderr
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		manager.setError(fmt.Errorf("create logs directory: %w", err))
	} else {
		_ = os.Chmod(logsDir, 0o700)
		writer, err := newRotateWriter(
			filepath.Join(logsDir, LogFileName),
			DefaultMaxBytes,
			DefaultBackups,
			manager.markUnavailable,
		)
		if err != nil {
			manager.setError(err)
		} else {
			output = writer
			manager.writer = writer
			manager.available.Store(true)
		}
	}

	handler := slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: &manager.level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				if value, ok := attr.Value.Any().(time.Time); ok {
					return slog.String(slog.TimeKey, value.UTC().Format(time.RFC3339Nano))
				}
			}
			return attr
		},
	})
	manager.logger = slog.New(handler)
	return manager
}

// Logger returns the structured logger.
func (m *Manager) Logger() *slog.Logger {
	return m.logger
}

// Available reports whether fixed-path disk logging is active.
func (m *Manager) Available() bool {
	return m.available.Load()
}

// InitError returns the safe disk initialization error, if any.
func (m *Manager) InitError() error {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()
	return m.initErr
}

// SetAvailabilitySink publishes fixed-path disk logging availability changes.
func (m *Manager) SetAvailabilitySink(sink func(bool)) {
	m.sinkMu.Lock()
	defer m.sinkMu.Unlock()
	m.sink = sink
	if sink != nil {
		sink(m.Available())
	}
}

// SetLevel atomically changes the runtime filter.
func (m *Manager) SetLevel(level string) error {
	parsed, err := ParseLevel(level)
	if err != nil {
		return err
	}
	m.level.Set(parsed)
	return nil
}

// Close flushes and closes the active log file.
func (m *Manager) Close() error {
	if m.writer == nil {
		return nil
	}
	return m.writer.Close()
}

// ParseLevel maps the public configuration values to slog levels.
func ParseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}

// Event adds the stable event field used by every runtime log.
func Event(ctx context.Context, logger *slog.Logger, level slog.Level, event string, attrs ...any) {
	args := make([]any, 0, len(attrs)+2)
	args = append(args, "event", event)
	args = append(args, attrs...)
	logger.Log(ctx, level, event, args...)
}

type rotateWriter struct {
	mu        sync.Mutex
	path      string
	maxBytes  int64
	backups   int
	file      *os.File
	size      int64
	failed    error
	onFailure func(error)
}

func newRotateWriter(
	path string,
	maxBytes int64,
	backups int,
	onFailure ...func(error),
) (*rotateWriter, error) {
	writer := &rotateWriter{path: path, maxBytes: maxBytes, backups: backups}
	if len(onFailure) > 0 {
		writer.onFailure = onFailure[0]
	}
	if err := writer.open(); err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return writer, nil
}

func (w *rotateWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.failed != nil {
		return 0, w.failed
	}
	if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			w.disable(err)
			return 0, err
		}
	}
	written, err := w.file.Write(data)
	w.size += int64(written)
	if err != nil {
		w.disable(err)
	}
	return written, err
}

func (w *rotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotateWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_ = os.Chmod(w.path, 0o600)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *rotateWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil

	oldest := fmt.Sprintf("%s.%d", w.path, w.backups)
	_ = os.Remove(oldest)
	for index := w.backups - 1; index >= 1; index-- {
		source := fmt.Sprintf("%s.%d", w.path, index)
		target := fmt.Sprintf("%s.%d", w.path, index+1)
		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.open()
}

func (w *rotateWriter) disable(err error) {
	if w.failed != nil {
		return
	}
	w.failed = err
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.onFailure != nil {
		w.onFailure(err)
	}
}

func (m *Manager) setError(err error) {
	m.errorMu.Lock()
	m.initErr = err
	m.errorMu.Unlock()
}

func (m *Manager) markUnavailable(err error) {
	if !m.available.Swap(false) {
		return
	}
	m.setError(fmt.Errorf("write log file: %w", err))
	m.sinkMu.Lock()
	defer m.sinkMu.Unlock()
	if m.sink != nil {
		m.sink(false)
	}
}
