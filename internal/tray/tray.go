// Package tray provides the native system-tray adapter.
package tray

import (
	"fmt"
	"sync"

	"fyne.io/systray"

	"github.com/zhoux77899/kitsune-proxy/internal/app"
	"github.com/zhoux77899/kitsune-proxy/internal/i18n"
)

// Actions contains the user actions exposed by the tray menu.
type Actions struct {
	OpenConfig   func()
	Reload       func()
	OpenLogs     func()
	SetAutostart func(bool) error
}

// Native owns the platform tray menu and translates application status.
type Native struct {
	catalog *i18n.Catalog

	mu               sync.Mutex
	ready            bool
	status           app.Status
	platformIconKey  string
	autostartEnabled bool
	statusItem       *systray.MenuItem
	modelsItem       *systray.MenuItem
	loggingItem      *systray.MenuItem
	autostartItem    *systray.MenuItem
}

type trayHealth string

const (
	trayHealthy  trayHealth = "healthy"
	trayDegraded trayHealth = "degraded"
	trayError    trayHealth = "error"
	trayStopped  trayHealth = "stopped"
)

type platformPresentation struct {
	icon     []byte
	iconKey  string
	template bool
	title    string
	tooltip  string
}

// New returns an unstarted tray adapter.
func New(catalog *i18n.Catalog, autostartEnabled bool) *Native {
	return &Native{
		catalog:          catalog,
		autostartEnabled: autostartEnabled,
		status: app.Status{
			State: app.StateStarting,
		},
	}
}

// Run starts the native event loop on the calling goroutine.
func (n *Native) Run(actions Actions, onExit func()) {
	systray.Run(func() {
		n.onReady(actions)
	}, onExit)
}

// Quit requests a clean exit from the native event loop.
func (n *Native) Quit() {
	systray.Quit()
}

// SetStatus updates or queues the latest safe application status.
func (n *Native) SetStatus(status app.Status) {
	n.mu.Lock()
	n.status = status
	if !n.ready {
		n.mu.Unlock()
		return
	}
	n.applyStatusLocked()
	n.mu.Unlock()
}

func (n *Native) onReady(actions Actions) {
	n.mu.Lock()
	n.applyPlatformPresentationLocked(true)
	n.statusItem = systray.AddMenuItem("", "")
	n.statusItem.Disable()
	n.modelsItem = systray.AddMenuItem("", "")
	n.modelsItem.Disable()
	n.loggingItem = systray.AddMenuItem(n.catalog.Text("menu_logging_unavailable"), "")
	n.loggingItem.Disable()
	systray.AddSeparator()
	openConfig := systray.AddMenuItem(n.catalog.Text("menu_open_config"), "")
	reload := systray.AddMenuItem(n.catalog.Text("menu_reload"), "")
	openLogs := systray.AddMenuItem(n.catalog.Text("menu_open_logs"), "")
	n.autostartItem = systray.AddMenuItemCheckbox(
		n.catalog.Text("menu_autostart"),
		"",
		n.autostartEnabled,
	)
	systray.AddSeparator()
	quit := systray.AddMenuItem(n.catalog.Text("menu_quit"), "")
	n.ready = true
	n.applyStatusLocked()
	n.mu.Unlock()

	go func() {
		for {
			select {
			case <-openConfig.ClickedCh:
				if actions.OpenConfig != nil {
					go actions.OpenConfig()
				}
			case <-reload.ClickedCh:
				if actions.Reload != nil {
					go actions.Reload()
				}
			case <-openLogs.ClickedCh:
				if actions.OpenLogs != nil {
					go actions.OpenLogs()
				}
			case <-n.autostartItem.ClickedCh:
				if actions.SetAutostart == nil {
					continue
				}
				n.mu.Lock()
				enabled := !n.autostartEnabled
				n.mu.Unlock()
				if err := actions.SetAutostart(enabled); err != nil {
					continue
				}
				n.mu.Lock()
				n.autostartEnabled = enabled
				n.applyAutostartLocked()
				n.mu.Unlock()
			case <-quit.ClickedCh:
				n.Quit()
				return
			}
		}
	}()
}

func (n *Native) applyStatusLocked() {
	n.applyPlatformPresentationLocked(false)
	n.statusItem.SetTitle(statusTitle(n.catalog, n.status))
	n.modelsItem.SetTitle(fmt.Sprintf(n.catalog.Text("menu_models"), n.status.Models))
	if n.status.LoggingAvailable {
		n.loggingItem.Hide()
	} else {
		n.loggingItem.Show()
	}
	n.applyAutostartLocked()
}

func (n *Native) applyPlatformPresentationLocked(initial bool) {
	presentation := platformPresentationFor(n.status, n.catalog.Text("tooltip"))
	if initial {
		if presentation.title != "" {
			systray.SetTitle(presentation.title)
		}
		if presentation.tooltip != "" {
			systray.SetTooltip(presentation.tooltip)
		}
	}
	if presentation.iconKey == n.platformIconKey {
		return
	}
	if presentation.template {
		systray.SetTemplateIcon(presentation.icon, presentation.icon)
	} else {
		systray.SetIcon(presentation.icon)
	}
	n.platformIconKey = presentation.iconKey
}

func (n *Native) applyAutostartLocked() {
	if n.autostartItem == nil {
		return
	}
	if n.autostartEnabled {
		n.autostartItem.Check()
	} else {
		n.autostartItem.Uncheck()
	}
}

func statusTitle(catalog *i18n.Catalog, status app.Status) string {
	switch status.State {
	case app.StateRunning:
		return fmt.Sprintf(catalog.Text("menu_status_running"), status.Address)
	case app.StateConfigError:
		if status.Address != "" {
			return fmt.Sprintf(catalog.Text("menu_status_config_error_at"), status.Address)
		}
		return catalog.Text("menu_status_config_error")
	case app.StateListenerError:
		if status.Address != "" {
			return fmt.Sprintf(catalog.Text("menu_status_listener_error_at"), status.Address)
		}
		return catalog.Text("menu_status_listener_error")
	case app.StateStopped:
		return catalog.Text("menu_status_stopped")
	default:
		return catalog.Text("menu_status_starting")
	}
}

func trayHealthForStatus(status app.Status) trayHealth {
	switch status.State {
	case app.StateStopped:
		return trayStopped
	case app.StateStarting:
		return trayDegraded
	case app.StateRunning:
		if status.Address == "" {
			return trayError
		}
		if !status.LoggingAvailable {
			return trayDegraded
		}
		return trayHealthy
	case app.StateConfigError:
		if status.Address != "" {
			return trayDegraded
		}
		return trayError
	case app.StateListenerError:
		return trayError
	default:
		return trayError
	}
}
