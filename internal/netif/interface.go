package netif

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// NetInterface represents a network interface with its connection details.
type NetInterface struct {
	Name         string
	FriendlyName string
	IP           net.IP
	Gateway      string
	NetworkName  string
	Luid         uint64
	Alive        bool
	mu           sync.RWMutex
}

// IsAlive checks if the interface can reach the internet by dialing a DNS server.
func (n *NetInterface) IsAlive() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Alive
}

// SetAlive updates the alive status.
func (n *NetInterface) SetAlive(alive bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Alive = alive
}

// SetNetworkName updates the connected network name.
func (n *NetInterface) SetNetworkName(name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.NetworkName = name
}

// NetworkNameValue returns the current network name.
func (n *NetInterface) NetworkNameValue() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.NetworkName
}

// String returns a readable representation.
func (n *NetInterface) String() string {
	status := "UP"
	if !n.IsAlive() {
		status = "DOWN"
	}
	name := n.FriendlyName
	if name == "" {
		name = n.Name
	}
	return fmt.Sprintf("%s (%s) [%s]", name, n.IP.String(), status)
}

func isUsableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return false
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	return true
}

func usableIPv4Addrs(addrs []net.Addr) []net.IP {
	var result []net.IP
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.To4() == nil {
			continue
		}

		result = append(result, ip.To4())
	}
	return result
}

// Discover enumerates all active network interfaces with valid IPv4 addresses.
func Discover() ([]*NetInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	var result []*NetInterface
	for _, iface := range ifaces {
		if !isUsableInterface(iface) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, ip := range usableIPv4Addrs(addrs) {
			ni := &NetInterface{
				Name:         iface.Name,
				FriendlyName: iface.Name,
				IP:           ip,
				Alive:        true,
			}
			result = append(result, ni)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no active network interfaces found")
	}

	// Populate friendly names, gateways, and interface IDs (platform-specific)
	populateDetails(result)
	RefreshNetworkNames(result)

	return result, nil
}

// CheckHealth tests if a network interface can reach the internet
// by binding to its IP and connecting to a DNS server.
func CheckHealth(ni *NetInterface) bool {
	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		LocalAddr: &net.TCPAddr{IP: ni.IP},
	}
	conn, err := dialer.Dial("tcp", "1.1.1.1:443")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

