package tray

import "github.com/ayanacorp/proxymon/internal/app"

type dashboardView interface {
	Show(app.Snapshot)
	Update(app.Snapshot)
	Close()
}
