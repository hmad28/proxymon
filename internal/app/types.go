package app

import (
	"time"

	"github.com/ayanacorp/proxymon/internal/balancer"
)

// Config describes the persisted and active runtime configuration.
type Config struct {
	ProxyAddr             string
	SelectedInterfaceKeys []string
	Mode                  balancer.Mode
	WinProxyAuto          bool
}

// InterfaceSnapshot is a read-only view of an interface for the tray UI.
type InterfaceSnapshot struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	FriendlyName string `json:"friendly_name"`
	IP           string `json:"ip"`
	Gateway      string `json:"gateway"`
	NetworkName  string `json:"network_name"`
	Alive        bool   `json:"alive"`
	Selected     bool   `json:"selected"`
	BytesSent    uint64 `json:"bytes_sent"`
	BytesRecv    uint64 `json:"bytes_recv"`
	RateSent     uint64 `json:"rate_sent"`
	RateRecv     uint64 `json:"rate_recv"`
}

// Snapshot is a read-only view of the current runtime state.
type Snapshot struct {
	Running           bool                `json:"running"`
	Version           string              `json:"version"`
	Uptime            time.Duration       `json:"uptime"`
	ProxyAddr         string              `json:"proxy_addr"`
	Socks5Addr        string              `json:"socks5_addr"`
	Mode              balancer.Mode       `json:"mode"`
	WinProxyAuto      bool                `json:"win_proxy_auto"`
	ActiveInterfaces  int                 `json:"active_interfaces"`
	BytesSent         uint64              `json:"bytes_sent"`
	BytesRecv         uint64              `json:"bytes_recv"`
	RateSent          uint64              `json:"rate_sent"`
	RateRecv          uint64              `json:"rate_recv"`
	ActiveConnections int64               `json:"active_connections"`
	TotalConnections  int64               `json:"total_connections"`
	Interfaces        []InterfaceSnapshot `json:"interfaces"`
	LastError         string              `json:"last_error"`
}

// Event is an asynchronous notification from the runtime.
type Event struct {
	Title   string
	Message string
}
