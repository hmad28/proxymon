//go:build windows && !cgo

package tray

import "github.com/ayanacorp/proxymon/internal/app"

func newDashboard(controller *app.Controller) (dashboardView, error) {
	_ = controller
	return newDashboardWindow()
}
