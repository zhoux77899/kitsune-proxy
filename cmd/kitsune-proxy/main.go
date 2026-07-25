package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"

	"github.com/zhoux77899/kitsune-proxy/internal/app"
	"github.com/zhoux77899/kitsune-proxy/internal/autostart"
	"github.com/zhoux77899/kitsune-proxy/internal/config"
	"github.com/zhoux77899/kitsune-proxy/internal/i18n"
	"github.com/zhoux77899/kitsune-proxy/internal/logging"
	"github.com/zhoux77899/kitsune-proxy/internal/tray"
)

func main() {
	paths, err := config.DefaultPaths()
	if err != nil {
		return
	}

	logs := logging.New(paths.LogsDir)
	catalog := i18n.System()
	controller := app.New(paths, logs, catalog, app.Options{})
	autostartEnabled, err := autostart.Enabled()
	if err != nil {
		logging.Event(context.Background(), logs.Logger(), slog.LevelError, "autostart_status_failed",
			"error", err.Error())
	}
	nativeTray := tray.New(catalog, autostartEnabled)
	controller.SetStatusSink(nativeTray.SetStatus)
	controller.Start()

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			controller.Shutdown()
			if err := logs.Close(); err != nil {
				_, _ = os.Stderr.WriteString(err.Error() + "\n")
			}
		})
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	go func() {
		<-signals
		nativeTray.Quit()
	}()

	open := func(path string, event string) func() {
		return func() {
			if err := tray.OpenPath(path); err != nil {
				logging.Event(context.Background(), logs.Logger(), slog.LevelError, event,
					"error", err.Error())
			}
		}
	}

	nativeTray.Run(tray.Actions{
		OpenConfig: open(paths.ConfigFile, "open_config_failed"),
		Reload: func() {
			_ = controller.Reload()
		},
		OpenLogs: open(paths.LogsDir, "open_logs_failed"),
		SetAutostart: func(enabled bool) error {
			if err := autostart.SetEnabled(enabled); err != nil {
				logging.Event(context.Background(), logs.Logger(), slog.LevelError, "autostart_change_failed",
					"enabled", enabled, "error", err.Error())
				return err
			}
			logging.Event(context.Background(), logs.Logger(), slog.LevelInfo, "autostart_changed",
				"enabled", enabled)
			return nil
		},
	}, shutdown)
	shutdown()
}
