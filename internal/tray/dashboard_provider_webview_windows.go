//go:build windows && cgo

package tray

import (
	"github.com/ayanacorp/proxymon/internal/app"
	"github.com/ayanacorp/proxymon/internal/dashboard"
)

func newDashboard(controller *app.Controller) (dashboardView, error) {
	return dashboard.New(controller)
}
