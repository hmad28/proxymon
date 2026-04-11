package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ayanacorp/proxymon/internal/balancer"
	cfg "github.com/ayanacorp/proxymon/internal/config"
	"github.com/ayanacorp/proxymon/internal/netif"
	"github.com/ayanacorp/proxymon/internal/proxy"
	"github.com/ayanacorp/proxymon/internal/winproxy"
)

type Controller struct {
	mu              sync.RWMutex
	store           *cfg.Store
	allIfaces       []*netif.NetInterface
	config          Config
	bal             balancer.Strategy
	proxyServer     *proxy.Server
	proxyBackup     *winproxy.ProxySettings
	runCancel       context.CancelFunc
	systemTraffic   *netif.SystemTrafficTracker
	perIfaceTracker *netif.PerInterfaceTracker
	lastError       error
	started         bool
	startedAt       time.Time
	version         string
	events          chan Event
}

func NewController(proxyAddr string, store *cfg.Store, version string) (*Controller, error) {
	ifaces, err := netif.Discover()
	if err != nil {
		return nil, err
	}

	c := &Controller{
		store:     store,
		allIfaces: ifaces,
		config: Config{
			ProxyAddr: proxyAddr,
			Mode:      balancer.ModeRoundRobin,
		},
		startedAt: time.Now(),
		version:   version,
		events:    make(chan Event, 16),
	}

	settings, err := store.Load()
	if err != nil {
		return nil, err
	}

	c.applyLoadedSettings(proxyAddr, settings)
	if err := c.startLocked(); err != nil {
		return nil, err
	}
	if err := c.persistLocked(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Controller) Events() <-chan Event {
	return c.events
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snap := Snapshot{
		Running:      c.started,
		Version:      c.version,
		Uptime:       time.Since(c.startedAt),
		ProxyAddr:    c.config.ProxyAddr,
		Mode:         c.config.Mode,
		WinProxyAuto: c.config.WinProxyAuto,
	}

	if c.proxyServer != nil {
		snap.Socks5Addr = c.proxyServer.Socks5Addr()
		stats := c.proxyServer.GetStats()
		snap.ActiveConnections = stats.ActiveConnections
		snap.TotalConnections = stats.TotalConnections
	}

	if c.systemTraffic != nil {
		system := c.systemTraffic.Snapshot()
		snap.ActiveInterfaces = system.ActiveInterfaces
		snap.BytesSent = system.BytesSent
		snap.BytesRecv = system.BytesRecv
		snap.RateSent = system.RateSent
		snap.RateRecv = system.RateRecv
	}

	if c.lastError != nil {
		snap.LastError = c.lastError.Error()
	}

	selected := make(map[string]bool, len(c.config.SelectedInterfaceKeys))
	for _, key := range c.config.SelectedInterfaceKeys {
		selected[key] = true
	}

	for _, iface := range c.allIfaces {
		key := InterfaceKey(iface)
		ifaceTraffic := netif.InterfaceTrafficSnapshot{}
		if c.perIfaceTracker != nil && iface.Luid != 0 {
			ifaceTraffic = c.perIfaceTracker.Get(iface.Luid)
		}

		snap.Interfaces = append(snap.Interfaces, InterfaceSnapshot{
			Key:          key,
			Name:         iface.Name,
			FriendlyName: iface.FriendlyName,
			IP:           iface.IP.String(),
			Gateway:      iface.Gateway,
			NetworkName:  iface.NetworkNameValue(),
			Alive:        iface.IsAlive(),
			Selected:     selected[key],
			BytesSent:    ifaceTraffic.BytesSent,
			BytesRecv:    ifaceTraffic.BytesRecv,
			RateSent:     ifaceTraffic.RateSent,
			RateRecv:     ifaceTraffic.RateRecv,
		})
	}

	return snap
}

func (c *Controller) Config() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneConfig(c.config)
}

func (c *Controller) ApplyConfig(next Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	next = c.normalizeConfig(next)
	if err := c.restartLocked(next); err != nil {
		c.lastError = err
		return err
	}

	if err := c.persistLocked(); err != nil {
		c.lastError = err
		return err
	}

	c.lastError = nil
	return nil
}

func (c *Controller) ResetStats() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.systemTraffic != nil {
		if err := c.systemTraffic.ResetBaseline(); err != nil {
			c.lastError = err
			return err
		}
	}
	if c.perIfaceTracker != nil {
		if err := c.perIfaceTracker.ResetBaseline(c.allInterfaceLUIDsLocked()); err != nil {
			c.lastError = err
			return err
		}
	}

	c.lastError = nil
	return nil
}

func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	c.started = false
	return nil
}

func (c *Controller) EmergencyDisableProxy() {
	_ = winproxy.Disable()
}

func InterfaceKey(iface *netif.NetInterface) string {
	return iface.Name
}

func DisplayName(iface *netif.NetInterface) string {
	if iface.FriendlyName != "" {
		return iface.FriendlyName
	}
	return iface.Name
}

func canonicalizeInterfaceKey(key string) string {
	key = strings.TrimSpace(key)
	if idx := strings.IndexByte(key, '|'); idx >= 0 {
		key = key[:idx]
	}
	return strings.TrimSpace(key)
}

func (c *Controller) applyLoadedSettings(proxyAddr string, settings cfg.Settings) {
	next := Config{
		ProxyAddr:             proxyAddr,
		SelectedInterfaceKeys: settings.SelectedInterfaceKeys,
		Mode:                  settings.Mode,
		WinProxyAuto:          settings.WinProxyAuto,
	}
	if settings.ProxyAddr != "" {
		next.ProxyAddr = settings.ProxyAddr
	}
	c.config = c.normalizeConfig(next)
}

func (c *Controller) normalizeConfig(next Config) Config {
	if next.ProxyAddr == "" {
		next.ProxyAddr = c.config.ProxyAddr
	}

	seen := make(map[string]bool)
	selectedSeen := make(map[string]bool)
	var valid []string
	for _, iface := range c.allIfaces {
		key := InterfaceKey(iface)
		seen[key] = true
	}
	for _, key := range next.SelectedInterfaceKeys {
		key = canonicalizeInterfaceKey(key)
		if key == "" || !seen[key] || selectedSeen[key] {
			continue
		}
		selectedSeen[key] = true
		valid = append(valid, key)
	}
	if len(valid) == 0 && len(c.allIfaces) > 0 {
		valid = []string{InterfaceKey(c.allIfaces[0])}
	}
	next.SelectedInterfaceKeys = valid

	if len(valid) == 1 {
		next.Mode = balancer.ModeFailover
	}
	if next.Mode != balancer.ModeFailover {
		next.Mode = balancer.ModeRoundRobin
	}

	return cloneConfig(next)
}

func (c *Controller) restartLocked(next Config) error {
	previous := cloneConfig(c.config)
	c.stopLocked()
	c.config = next
	if err := c.startLocked(); err != nil {
		c.config = previous
		if restoreErr := c.startLocked(); restoreErr != nil {
			return fmt.Errorf("apply failed: %w (restore failed: %v)", err, restoreErr)
		}
		return err
	}
	return nil
}

func (c *Controller) startLocked() error {
	selected := c.selectedIfacesLocked()
	if len(selected) == 0 {
		return fmt.Errorf("no network interfaces selected")
	}

	allLUIDs := c.allInterfaceLUIDsLocked()
	runCtx, cancel := context.WithCancel(context.Background())
	c.runCancel = cancel
	c.bal = balancer.New(c.config.Mode, selected)
	c.proxyServer = proxy.NewServer(c.config.ProxyAddr, c.bal)
	c.systemTraffic = netif.NewSystemTrafficTracker()
	c.perIfaceTracker = netif.NewPerInterfaceTracker()
	if err := c.systemTraffic.ResetBaseline(); err != nil {
		cancel()
		c.runCancel = nil
		c.systemTraffic = nil
		c.perIfaceTracker = nil
		c.bal = nil
		c.proxyServer = nil
		return err
	}
	if err := c.perIfaceTracker.ResetBaseline(allLUIDs); err != nil {
		cancel()
		c.runCancel = nil
		c.systemTraffic = nil
		c.perIfaceTracker = nil
		c.bal = nil
		c.proxyServer = nil
		return err
	}

	if c.config.WinProxyAuto {
		backup, err := winproxy.Backup()
		if err != nil {
			cancel()
			c.runCancel = nil
			c.systemTraffic = nil
			c.perIfaceTracker = nil
			c.bal = nil
			c.proxyServer = nil
			return err
		}
		c.proxyBackup = backup
		if err := winproxy.Enable(c.config.ProxyAddr); err != nil {
			cancel()
			c.runCancel = nil
			c.proxyBackup = nil
			c.systemTraffic = nil
			c.perIfaceTracker = nil
			c.bal = nil
			c.proxyServer = nil
			return err
		}
	}

	go c.watchInterfaces(runCtx)
	go c.systemTraffic.Monitor(runCtx, 2*time.Second)
	go c.perIfaceTracker.Monitor(runCtx, 2*time.Second)
	go func(server *proxy.Server, ctx context.Context) {
		if err := server.Start(ctx); err != nil {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.runCancel == nil || ctx.Err() != nil {
				return
			}
			c.lastError = err
			c.sendEventLocked("Proxy stopped", err.Error())
		}
	}(c.proxyServer, runCtx)

	c.started = true
	return nil
}

func (c *Controller) stopLocked() {
	if c.runCancel != nil {
		c.runCancel()
		c.runCancel = nil
	}
	if c.proxyServer != nil {
		c.proxyServer.Stop()
		c.proxyServer = nil
	}
	if c.proxyBackup != nil {
		_ = winproxy.Restore(c.proxyBackup)
		c.proxyBackup = nil
	}
	c.systemTraffic = nil
	c.perIfaceTracker = nil
	c.bal = nil
	c.started = false
}

func (c *Controller) selectedIfacesLocked() []*netif.NetInterface {
	selected := make(map[string]bool, len(c.config.SelectedInterfaceKeys))
	for _, key := range c.config.SelectedInterfaceKeys {
		selected[key] = true
	}

	var result []*netif.NetInterface
	for _, iface := range c.allIfaces {
		if selected[InterfaceKey(iface)] {
			result = append(result, iface)
		}
	}
	return result
}

func (c *Controller) allInterfaceLUIDsLocked() []uint64 {
	seen := make(map[uint64]bool)
	luids := make([]uint64, 0, len(c.allIfaces))
	for _, iface := range c.allIfaces {
		if iface.Luid == 0 || seen[iface.Luid] {
			continue
		}
		seen[iface.Luid] = true
		luids = append(luids, iface.Luid)
	}
	return luids
}

func (c *Controller) persistLocked() error {
	return c.store.Save(cfg.Settings{
		ProxyAddr:             c.config.ProxyAddr,
		SelectedInterfaceKeys: append([]string(nil), c.config.SelectedInterfaceKeys...),
		Mode:                  c.config.Mode,
		WinProxyAuto:          c.config.WinProxyAuto,
	})
}

func (c *Controller) sendEventLocked(title, message string) {
	select {
	case c.events <- Event{Title: title, Message: message}:
	default:
	}
}

func cloneConfig(cfg Config) Config {
	cfg.SelectedInterfaceKeys = append([]string(nil), cfg.SelectedInterfaceKeys...)
	return cfg
}

func (c *Controller) watchInterfaces(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ifaces, err := netif.Discover()
			if err != nil {
				continue
			}

			netif.RefreshNetworkNames(ifaces)
			for _, ni := range ifaces {
				ni.SetAlive(netif.CheckHealth(ni))
			}

			c.mu.Lock()
			c.allIfaces = ifaces

			selected := c.selectedIfacesLocked()
			allLUIDs := c.allInterfaceLUIDsLocked()

			if c.bal != nil {
				c.bal.SetInterfaces(selected)
			}

			if c.perIfaceTracker != nil {
				c.perIfaceTracker.UpdateLUIDs(allLUIDs)
			}
			c.mu.Unlock()
		}
	}
}
