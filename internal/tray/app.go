package tray

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/getlantern/systray"

	"github.com/ayanacorp/proxymon/internal/app"
)

//go:embed icon.ico
var trayIcon []byte

type App struct {
	controller *app.Controller
	menu       *menuState
	dashboard  dashboardView
	done       chan struct{}
}

func New(controller *app.Controller) *App {
	return &App{controller: controller, done: make(chan struct{})}
}

func (a *App) Run() {
	systray.Run(a.onReady, a.onExit)
}

func (a *App) onReady() {
	if len(trayIcon) > 0 {
		systray.SetIcon(trayIcon)
	}

	var err error
	a.dashboard, err = newDashboard(a.controller)
	if err != nil {
		showPopup("Dashboard init failed", err.Error())
	}

	snapshot := a.controller.Snapshot()
	systray.SetTooltip(fmt.Sprintf("Proxymon\n↑ %s/s  ↓ %s/s", formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))
	if a.dashboard != nil {
		a.dashboard.Update(snapshot)
	}

	systray.SetTrayLeftClick(func() {
		a.openDashboard()
	})

	a.menu = newMenuState(a)
	a.menu.build()
	a.menu.refresh(snapshot)
	go a.menu.handleClicks(a.done)
	go a.runTooltipLoop()
	go a.runEventLoop()
}

func (a *App) onExit() {
	select {
	case <-a.done:
	default:
		close(a.done)
	}
	if a.dashboard != nil {
		a.dashboard.Close()
	}
	_ = a.controller.Stop()
}

func (a *App) openDashboard() {
	if a.dashboard == nil {
		showPopup("Dashboard unavailable", "Dashboard window could not be created.")
		return
	}
	a.dashboard.Show(a.controller.Snapshot())
}

func (a *App) runTooltipLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			snapshot := a.controller.Snapshot()
			systray.SetTooltip(fmt.Sprintf("Proxymon\n↑ %s/s  ↓ %s/s", formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))
			a.menu.refresh(snapshot)
			if a.dashboard != nil {
				a.dashboard.Update(snapshot)
			}
		}
	}
}

func (a *App) runEventLoop() {
	events := a.controller.Events()
	for {
		select {
		case <-a.done:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			showPopup(event.Title, event.Message)
		}
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
