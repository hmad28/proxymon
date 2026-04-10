package tray

import (
	"fmt"

	"github.com/getlantern/systray"

	"github.com/ayanacorp/proxymon/internal/app"
	"github.com/ayanacorp/proxymon/internal/balancer"
)

type menuState struct {
	app        *App
	controller *app.Controller
	interfaces map[string]*systray.MenuItem
	roundRobin *systray.MenuItem
	failover   *systray.MenuItem
	autoProxy  *systray.MenuItem
	resetStats *systray.MenuItem
	showStats  *systray.MenuItem
	quit       *systray.MenuItem
	status     *systray.MenuItem
}

func newMenuState(application *App) *menuState {
	return &menuState{
		app:        application,
		controller: application.controller,
		interfaces: make(map[string]*systray.MenuItem),
	}
}

func (m *menuState) build() {
	snapshot := m.controller.Snapshot()
	m.status = systray.AddMenuItem("Running…", "Current proxy status")
	m.status.Disable()
	systray.AddSeparator()

	interfacesRoot := systray.AddMenuItem("Interfaces", "Select interfaces")
	for _, iface := range snapshot.Interfaces {
		label := iface.FriendlyName
		if label == "" {
			label = iface.Name
		}
		item := interfacesRoot.AddSubMenuItemCheckbox(fmt.Sprintf("%s (%s)", label, iface.IP), "Toggle interface", iface.Selected)
		m.interfaces[iface.Key] = item
	}

	modeRoot := systray.AddMenuItem("Balancing Mode", "Select balancing mode")
	m.roundRobin = modeRoot.AddSubMenuItemCheckbox("Round-Robin", "Use round-robin balancing", snapshot.Mode == balancer.ModeRoundRobin)
	m.failover = modeRoot.AddSubMenuItemCheckbox("Failover", "Use failover balancing", snapshot.Mode == balancer.ModeFailover)

	systray.AddSeparator()
	m.autoProxy = systray.AddMenuItemCheckbox("Enable Windows auto-proxy", "Toggle Windows system proxy configuration", snapshot.WinProxyAuto)
	m.resetStats = systray.AddMenuItem("Reset stats", "Reset interface and global counters")
	m.showStats = systray.AddMenuItem("Show dashboard", "Open the live dashboard")
	systray.AddSeparator()
	m.quit = systray.AddMenuItem("Quit", "Stop the proxy and exit")

	m.refresh(snapshot)
}

func (m *menuState) handleClicks(done <-chan struct{}) {
	for key, item := range m.interfaces {
		go func(key string, item *systray.MenuItem) {
			for {
				select {
				case <-done:
					return
				case <-item.ClickedCh:
					m.toggleInterface(key)
				}
			}
		}(key, item)
	}

	go func() {
		for {
			select {
			case <-done:
				return
			case <-m.roundRobin.ClickedCh:
				m.setMode(balancer.ModeRoundRobin)
			case <-m.failover.ClickedCh:
				m.setMode(balancer.ModeFailover)
			case <-m.autoProxy.ClickedCh:
				m.toggleAutoProxy()
			case <-m.resetStats.ClickedCh:
				if err := m.controller.ResetStats(); err != nil {
					showPopup("Reset stats failed", err.Error())
				}
			case <-m.showStats.ClickedCh:
				m.app.openDashboard()
			case <-m.quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (m *menuState) refresh(snapshot app.Snapshot) {
	m.status.SetTitle(fmt.Sprintf("↑ %s/s  ↓ %s/s", formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))

	selectedCount := 0
	for _, iface := range snapshot.Interfaces {
		item := m.interfaces[iface.Key]
		if item == nil {
			continue
		}
		if iface.Selected {
			item.Check()
			selectedCount++
		} else {
			item.Uncheck()
		}
	}

	if snapshot.Mode == balancer.ModeRoundRobin {
		m.roundRobin.Check()
		m.failover.Uncheck()
	} else {
		m.roundRobin.Uncheck()
		m.failover.Check()
	}

	if snapshot.WinProxyAuto {
		m.autoProxy.Check()
	} else {
		m.autoProxy.Uncheck()
	}

	if selectedCount <= 1 {
		m.roundRobin.Disable()
	} else {
		m.roundRobin.Enable()
	}
}

func (m *menuState) toggleInterface(key string) {
	cfg := m.controller.Config()
	selected := make(map[string]bool, len(cfg.SelectedInterfaceKeys))
	for _, current := range cfg.SelectedInterfaceKeys {
		selected[current] = true
	}
	selected[key] = !selected[key]

	var next []string
	for _, iface := range m.controller.Snapshot().Interfaces {
		if selected[iface.Key] {
			next = append(next, iface.Key)
		}
	}
	if len(next) == 0 {
		showPopup("Selection required", "At least one interface must remain selected.")
		m.refresh(m.controller.Snapshot())
		return
	}

	cfg.SelectedInterfaceKeys = next
	if err := m.controller.ApplyConfig(cfg); err != nil {
		showPopup("Interface update failed", err.Error())
	}
	m.refresh(m.controller.Snapshot())
}

func (m *menuState) setMode(mode balancer.Mode) {
	cfg := m.controller.Config()
	cfg.Mode = mode
	if err := m.controller.ApplyConfig(cfg); err != nil {
		showPopup("Mode update failed", err.Error())
	}
	m.refresh(m.controller.Snapshot())
}

func (m *menuState) toggleAutoProxy() {
	cfg := m.controller.Config()
	cfg.WinProxyAuto = !cfg.WinProxyAuto
	if err := m.controller.ApplyConfig(cfg); err != nil {
		showPopup("Auto-proxy update failed", err.Error())
	}
	m.refresh(m.controller.Snapshot())
}
