package netif

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// SystemTrafficSnapshot contains aggregated system-wide traffic totals.
type SystemTrafficSnapshot struct {
	ActiveInterfaces int
	BytesSent        uint64
	BytesRecv        uint64
	RateSent         uint64
	RateRecv         uint64
}

type systemTrafficSample struct {
	bytesSent uint64
	bytesRecv uint64
}

// InterfaceTrafficSnapshot holds per-NIC traffic data.
type InterfaceTrafficSnapshot struct {
	BytesSent uint64
	BytesRecv uint64
	RateSent  uint64
	RateRecv  uint64
}

type perIfaceEntry struct {
	baseline   [2]uint64
	lastSample [2]uint64
	current    InterfaceTrafficSnapshot
}

type PerInterfaceTracker struct {
	mu          sync.RWMutex
	interval    time.Duration
	entries     map[uint64]*perIfaceEntry
	activeLUIDs []uint64
}

// NewSystemTrafficTracker creates a tracker for system-wide network traffic.
func NewSystemTrafficTracker() *SystemTrafficTracker {
	return &SystemTrafficTracker{}
}

// NewPerInterfaceTracker creates a tracker for per-interface traffic.
func NewPerInterfaceTracker() *PerInterfaceTracker {
	return &PerInterfaceTracker{entries: make(map[uint64]*perIfaceEntry)}
}

// SystemTrafficTracker monitors cumulative bytes across all active interfaces.
type SystemTrafficTracker struct {
	mu         sync.RWMutex
	baseline   systemTrafficSample
	lastSample systemTrafficSample
	current    SystemTrafficSnapshot
	interval   time.Duration
}

// Snapshot returns the latest aggregated snapshot.
func (t *SystemTrafficTracker) Snapshot() SystemTrafficSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.current
}

// ResetBaseline resets displayed totals to the current cumulative OS counters.
func (t *SystemTrafficTracker) ResetBaseline() error {
	totals, err := readSystemTrafficTotals()
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.baseline = systemTrafficSample{bytesSent: totals.BytesSent, bytesRecv: totals.BytesRecv}
	t.lastSample = systemTrafficSample{bytesSent: totals.BytesSent, bytesRecv: totals.BytesRecv}
	t.current = SystemTrafficSnapshot{ActiveInterfaces: totals.ActiveInterfaces}
	return nil
}

// Monitor periodically refreshes aggregated traffic totals.
func (t *SystemTrafficTracker) Monitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}

	t.mu.Lock()
	t.interval = interval
	t.mu.Unlock()

	if err := t.refresh(true); err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = t.refresh(false)
		}
	}
}

func (t *SystemTrafficTracker) refresh(setBaseline bool) error {
	totals, err := readSystemTrafficTotals()
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if setBaseline || totals.BytesSent < t.baseline.bytesSent || totals.BytesRecv < t.baseline.bytesRecv {
		t.baseline = systemTrafficSample{bytesSent: totals.BytesSent, bytesRecv: totals.BytesRecv}
	}
	if setBaseline || totals.BytesSent < t.lastSample.bytesSent || totals.BytesRecv < t.lastSample.bytesRecv {
		t.lastSample = systemTrafficSample{bytesSent: totals.BytesSent, bytesRecv: totals.BytesRecv}
	}

	rateSec := uint64(t.interval.Seconds())
	if rateSec < 1 {
		rateSec = 1
	}

	t.current = SystemTrafficSnapshot{
		ActiveInterfaces: totals.ActiveInterfaces,
		BytesSent:        totals.BytesSent - t.baseline.bytesSent,
		BytesRecv:        totals.BytesRecv - t.baseline.bytesRecv,
		RateSent:         (totals.BytesSent - t.lastSample.bytesSent) / rateSec,
		RateRecv:         (totals.BytesRecv - t.lastSample.bytesRecv) / rateSec,
	}
	t.lastSample = systemTrafficSample{bytesSent: totals.BytesSent, bytesRecv: totals.BytesRecv}
	return nil
}

// Get returns the latest snapshot for a specific interface LUID.
func (t *PerInterfaceTracker) Get(luid uint64) InterfaceTrafficSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	entry := t.entries[luid]
	if entry == nil {
		return InterfaceTrafficSnapshot{}
	}
	return entry.current
}

// ResetBaseline resets all tracked interface totals to the current OS counters.
func (t *PerInterfaceTracker) ResetBaseline(luids []uint64) error {
	samples, err := readInterfaceSamples(luids)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[uint64]*perIfaceEntry, len(luids))
	}
	t.activeLUIDs = append([]uint64(nil), luids...)
	for _, luid := range luids {
		sample, ok := samples[luid]
		if !ok {
			continue
		}
		entry := t.entries[luid]
		if entry == nil {
			entry = &perIfaceEntry{}
			t.entries[luid] = entry
		}
		entry.baseline = [2]uint64{sample[0], sample[1]}
		entry.lastSample = [2]uint64{sample[0], sample[1]}
		entry.current = InterfaceTrafficSnapshot{}
	}
	for luid := range t.entries {
		if !containsLUID(luids, luid) {
			delete(t.entries, luid)
		}
	}
	return nil
}

// UpdateLUIDs updates the monitored LUIDs dynamically.
func (t *PerInterfaceTracker) UpdateLUIDs(luids []uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeLUIDs = append([]uint64(nil), luids...)
}

// Monitor periodically refreshes per-interface traffic totals.
func (t *PerInterfaceTracker) Monitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}

	t.mu.Lock()
	t.interval = interval
	t.mu.Unlock()

	if err := t.refresh(true); err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = t.refresh(false)
		}
	}
}

func (t *PerInterfaceTracker) refresh(setBaseline bool) error {
	t.mu.RLock()
	luids := append([]uint64(nil), t.activeLUIDs...)
	t.mu.RUnlock()

	samples, err := readInterfaceSamples(luids)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[uint64]*perIfaceEntry, len(luids))
	}

	rateSec := uint64(t.interval.Seconds())
	if rateSec < 1 {
		rateSec = 1
	}

	for _, luid := range luids {
		sample, ok := samples[luid]
		if !ok {
			continue
		}
		entry := t.entries[luid]
		if entry == nil {
			entry = &perIfaceEntry{
				baseline:   [2]uint64{sample[0], sample[1]},
				lastSample: [2]uint64{sample[0], sample[1]},
			}
			t.entries[luid] = entry
		}

		if setBaseline || sample[0] < entry.baseline[0] || sample[1] < entry.baseline[1] {
			entry.baseline = [2]uint64{sample[0], sample[1]}
		}
		if setBaseline || sample[0] < entry.lastSample[0] || sample[1] < entry.lastSample[1] {
			entry.lastSample = [2]uint64{sample[0], sample[1]}
		}

		entry.current = InterfaceTrafficSnapshot{
			BytesSent: sample[0] - entry.baseline[0],
			BytesRecv: sample[1] - entry.baseline[1],
			RateSent:  (sample[0] - entry.lastSample[0]) / rateSec,
			RateRecv:  (sample[1] - entry.lastSample[1]) / rateSec,
		}
		entry.lastSample = [2]uint64{sample[0], sample[1]}
	}

	for luid := range t.entries {
		if !containsLUID(luids, luid) {
			delete(t.entries, luid)
		}
	}

	return nil
}

func readSystemTrafficTotals() (SystemTrafficSnapshot, error) {
	adapters, err := getActiveIPv4Adapters()
	if err != nil {
		return SystemTrafficSnapshot{}, err
	}

	var snapshot SystemTrafficSnapshot
	for _, adapter := range adapters {
		sample, err := readInterfaceSample(adapter.Luid)
		if err != nil {
			continue
		}
		snapshot.ActiveInterfaces++
		snapshot.BytesSent += sample[0]
		snapshot.BytesRecv += sample[1]
	}

	return snapshot, nil
}

func readInterfaceSamples(luids []uint64) (map[uint64][2]uint64, error) {
	samples := make(map[uint64][2]uint64, len(luids))
	var firstErr error
	for _, luid := range luids {
		sample, err := readInterfaceSample(luid)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		samples[luid] = sample
	}
	if len(luids) > 0 && len(samples) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return samples, nil
}

func readInterfaceSample(luid uint64) ([2]uint64, error) {
	row := windows.MibIfRow2{InterfaceLuid: luid}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row); err != nil {
		return [2]uint64{}, fmt.Errorf("get interface stats for LUID %d: %w", luid, err)
	}
	if row.Type == windows.IF_TYPE_SOFTWARE_LOOPBACK || row.OperStatus != windows.IfOperStatusUp {
		return [2]uint64{}, fmt.Errorf("interface LUID %d is not up", luid)
	}
	return [2]uint64{row.OutOctets, row.InOctets}, nil
}

func getActiveIPv4Adapters() ([]windows.IpAdapterAddresses, error) {
	first, err := getAdaptersAddresses(windows.GAA_FLAG_INCLUDE_PREFIX)
	if err != nil {
		return nil, fmt.Errorf("get adapters addresses: %w", err)
	}
	return collectActiveIPv4Adapters(first), nil
}

func collectActiveIPv4Adapters(first *windows.IpAdapterAddresses) []windows.IpAdapterAddresses {
	seen := make(map[uint64]bool)
	var adapters []windows.IpAdapterAddresses

	for aa := first; aa != nil; aa = aa.Next {
		if aa.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK || aa.OperStatus != windows.IfOperStatusUp {
			continue
		}
		if !hasUsableIPv4Address(aa.FirstUnicastAddress) {
			continue
		}
		if seen[aa.Luid] {
			continue
		}

		seen[aa.Luid] = true
		adapters = append(adapters, *aa)
	}

	return adapters
}

func hasUsableIPv4Address(addr *windows.IpAdapterUnicastAddress) bool {
	for ua := addr; ua != nil; ua = ua.Next {
		if usableIPv4FromSocketAddress(&ua.Address) != nil {
			return true
		}
	}
	return false
}

func containsLUID(luids []uint64, target uint64) bool {
	for _, luid := range luids {
		if luid == target {
			return true
		}
	}
	return false
}
