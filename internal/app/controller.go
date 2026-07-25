// Package app coordinates configuration, proxy serving, reload, and shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/config"
	"github.com/zhoux77899/kitsune-proxy/internal/logging"
	"github.com/zhoux77899/kitsune-proxy/internal/proxy"
	"github.com/zhoux77899/kitsune-proxy/internal/router"
)

// State is the high-level tray-visible application state.
type State string

const (
	StateStarting      State = "starting"
	StateRunning       State = "running"
	StateConfigError   State = "config_error"
	StateListenerError State = "listener_error"
	StateStopped       State = "stopped"
)

// Status is safe for UI display and never contains credentials.
type Status struct {
	State            State
	Address          string
	Models           int
	Detail           string
	LoggingAvailable bool
}

// Options exposes only the seams needed by production and integration tests.
type Options struct {
	Transport       http.RoundTripper
	Listen          func(network, address string) (net.Listener, error)
	ShutdownTimeout time.Duration
}

// Controller owns the active listener and the atomic proxy runtime.
type Controller struct {
	paths      config.Paths
	logs       *logging.Manager
	logger     *slog.Logger
	handler    *proxy.Handler
	listen     func(network, address string) (net.Listener, error)
	shutdownIn time.Duration

	reloadMu sync.Mutex
	mu       sync.Mutex
	current  *deployment
	stopping bool
	servers  sync.WaitGroup

	statusMu   sync.Mutex
	status     Status
	statusSink func(Status)
}

type deployment struct {
	config   config.Config
	runtime  *proxy.Runtime
	server   *http.Server
	listener net.Listener
}

// New creates a controller without starting disk or network work.
func New(paths config.Paths, logs *logging.Manager, localizer proxy.Localizer, options Options) *Controller {
	if options.Listen == nil {
		options.Listen = net.Listen
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 10 * time.Second
	}
	handler := proxy.New(nil, options.Transport, logs.Logger(), localizer)
	controller := &Controller{
		paths:      paths,
		logs:       logs,
		logger:     logs.Logger(),
		handler:    handler,
		listen:     options.Listen,
		shutdownIn: options.ShutdownTimeout,
		status: Status{
			State:            StateStarting,
			LoggingAvailable: logs.Available(),
		},
	}
	logs.SetAvailabilitySink(controller.setLoggingAvailable)
	return controller
}

// SetStatusSink installs the tray status adapter and immediately publishes state.
func (c *Controller) SetStatusSink(sink func(Status)) {
	c.statusMu.Lock()
	c.statusSink = sink
	status := c.status
	c.statusMu.Unlock()
	if sink != nil {
		sink(status)
	}
}

// Status returns the latest safe UI snapshot.
func (c *Controller) Status() Status {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	return c.status
}

// Start creates the first-run config and starts a valid listener when possible.
func (c *Controller) Start() {
	created, err := config.Ensure(c.paths)
	if err != nil {
		c.configFailure("config_create_failed", err)
		return
	}
	if created {
		logging.Event(context.Background(), c.logger, slog.LevelInfo, "config_created",
			"path", c.paths.ConfigFile)
	}
	if !c.logs.Available() {
		logging.Event(context.Background(), c.logger, slog.LevelError, "logging_unavailable",
			"error", safeError(c.logs.InitError()))
	}

	cfg, err := config.Load(c.paths.ConfigFile)
	if err != nil {
		c.configFailure("config_load_failed", err)
		return
	}
	if err := c.activateInitial(cfg); err != nil {
		c.listenerFailure("listener_start_failed", cfg.Server.Port, err)
	}
}

// Reload applies a complete configuration transaction.
func (c *Controller) Reload() error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	cfg, err := config.Load(c.paths.ConfigFile)
	if err != nil {
		logging.Event(context.Background(), c.logger, slog.LevelError, "config_reload_failed",
			"error", safeError(err))
		c.publishFailure(err)
		return err
	}
	runtimeSnapshot, err := buildRuntime(cfg)
	if err != nil {
		logging.Event(context.Background(), c.logger, slog.LevelError, "config_reload_failed",
			"error", safeError(err))
		c.publishFailure(err)
		return err
	}

	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return errors.New("application is stopping")
	}
	current := c.current
	c.mu.Unlock()

	if current != nil && current.config.Server.Port == cfg.Server.Port {
		c.mu.Lock()
		if c.stopping {
			c.mu.Unlock()
			return errors.New("application is stopping")
		}
		if c.current == current {
			c.handler.Update(runtimeSnapshot)
			current.config = cfg
			current.runtime = runtimeSnapshot
			_ = c.logs.SetLevel(cfg.Logging.Level)
			c.mu.Unlock()
			c.publishRunning(cfg, runtimeSnapshot, "")
			logging.Event(context.Background(), c.logger, slog.LevelInfo, "config_reload_succeeded",
				"port", cfg.Server.Port, "models", runtimeSnapshot.Table.Len())
			return nil
		}
		c.mu.Unlock()
	}

	next, err := c.prepareDeployment(cfg, runtimeSnapshot)
	if err != nil {
		logging.Event(context.Background(), c.logger, slog.LevelError, "config_reload_failed",
			"error", safeError(err))
		c.publishFailure(err)
		return err
	}

	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		_ = next.listener.Close()
		return errors.New("application is stopping")
	}
	previous := c.current
	c.handler.Update(runtimeSnapshot)
	_ = c.logs.SetLevel(cfg.Logging.Level)
	c.current = next
	c.startServingLocked(next)
	c.mu.Unlock()

	c.publishRunning(cfg, runtimeSnapshot, "")
	logging.Event(context.Background(), c.logger, slog.LevelInfo, "config_reload_succeeded",
		"port", cfg.Server.Port, "models", runtimeSnapshot.Table.Len())
	if previous != nil {
		go c.shutdownDeployment(previous)
	}
	return nil
}

// Shutdown gracefully stops the active listener. It is idempotent.
func (c *Controller) Shutdown() {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return
	}
	c.stopping = true
	current := c.current
	c.current = nil
	c.mu.Unlock()

	if current != nil {
		c.shutdownDeployment(current)
	}
	c.servers.Wait()
	c.publish(Status{State: StateStopped, LoggingAvailable: c.logs.Available()})
	logging.Event(context.Background(), c.logger, slog.LevelInfo, "app_stopped")
}

func (c *Controller) activateInitial(cfg config.Config) error {
	runtimeSnapshot, err := buildRuntime(cfg)
	if err != nil {
		return err
	}
	next, err := c.prepareDeployment(cfg, runtimeSnapshot)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.handler.Update(runtimeSnapshot)
	_ = c.logs.SetLevel(cfg.Logging.Level)
	c.current = next
	c.startServingLocked(next)
	c.mu.Unlock()

	c.publishRunning(cfg, runtimeSnapshot, "")
	logging.Event(context.Background(), c.logger, slog.LevelInfo, "app_started",
		"os", runtime.GOOS, "arch", runtime.GOARCH)
	logging.Event(context.Background(), c.logger, slog.LevelInfo, "listener_started",
		"address", listenerAddress(cfg.Server.Port), "models", runtimeSnapshot.Table.Len())
	return nil
}

func (c *Controller) prepareDeployment(cfg config.Config, runtimeSnapshot *proxy.Runtime) (*deployment, error) {
	listener, err := c.listen("tcp4", fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port))
	if err != nil {
		return nil, fmt.Errorf("bind %s: %w", listenerAddress(cfg.Server.Port), err)
	}
	server := &http.Server{
		Handler:           c.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	return &deployment{
		config: cfg, runtime: runtimeSnapshot, server: server, listener: listener,
	}, nil
}

func (c *Controller) startServingLocked(active *deployment) {
	c.servers.Add(1)
	go func() {
		defer c.servers.Done()
		err := active.server.Serve(active.listener)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		logging.Event(context.Background(), c.logger, slog.LevelError, "listener_failed",
			"address", listenerAddress(active.config.Server.Port), "error", safeError(err))
		c.mu.Lock()
		isCurrent := c.current == active && !c.stopping
		if isCurrent {
			c.current = nil
		}
		c.mu.Unlock()
		if isCurrent {
			c.publish(Status{
				State:            StateListenerError,
				Address:          listenerAddress(active.config.Server.Port),
				Models:           active.runtime.Table.Len(),
				Detail:           safeError(err),
				LoggingAvailable: c.logs.Available(),
			})
		}
	}()
}

func (c *Controller) shutdownDeployment(active *deployment) {
	ctx, cancel := context.WithTimeout(context.Background(), c.shutdownIn)
	defer cancel()
	if err := active.server.Shutdown(ctx); err != nil {
		_ = active.server.Close()
	}
}

func (c *Controller) configFailure(event string, err error) {
	logging.Event(context.Background(), c.logger, slog.LevelError, event, "error", safeError(err))
	c.publish(Status{
		State:            StateConfigError,
		Detail:           safeError(err),
		LoggingAvailable: c.logs.Available(),
	})
}

func (c *Controller) listenerFailure(event string, port int, err error) {
	logging.Event(context.Background(), c.logger, slog.LevelError, event,
		"address", listenerAddress(port), "error", safeError(err))
	c.publish(Status{
		State:            StateListenerError,
		Address:          listenerAddress(port),
		Detail:           safeError(err),
		LoggingAvailable: c.logs.Available(),
	})
}

func (c *Controller) publishFailure(err error) {
	c.mu.Lock()
	current := c.current
	c.mu.Unlock()
	if current == nil {
		c.publish(Status{
			State:            StateConfigError,
			Detail:           safeError(err),
			LoggingAvailable: c.logs.Available(),
		})
		return
	}
	c.publish(Status{
		State:            StateConfigError,
		Address:          listenerAddress(current.config.Server.Port),
		Models:           current.runtime.Table.Len(),
		Detail:           safeError(err),
		LoggingAvailable: c.logs.Available(),
	})
}

func (c *Controller) publishRunning(cfg config.Config, runtimeSnapshot *proxy.Runtime, detail string) {
	c.publish(Status{
		State:            StateRunning,
		Address:          listenerAddress(cfg.Server.Port),
		Models:           runtimeSnapshot.Table.Len(),
		Detail:           detail,
		LoggingAvailable: c.logs.Available(),
	})
}

func (c *Controller) publish(status Status) {
	c.statusMu.Lock()
	c.status = status
	sink := c.statusSink
	c.statusMu.Unlock()
	if sink != nil {
		sink(status)
	}
}

func (c *Controller) setLoggingAvailable(available bool) {
	c.statusMu.Lock()
	c.status.LoggingAvailable = available
	status := c.status
	sink := c.statusSink
	c.statusMu.Unlock()
	if sink != nil {
		sink(status)
	}
}

func buildRuntime(cfg config.Config) (*proxy.Runtime, error) {
	table, err := router.New(cfg)
	if err != nil {
		return nil, err
	}
	return &proxy.Runtime{LocalAPIKey: cfg.Server.APIKey, Table: table}, nil
}

func listenerAddress(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
