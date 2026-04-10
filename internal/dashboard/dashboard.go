//go:build windows && cgo

package dashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"github.com/webview/webview_go"
	"golang.org/x/sys/windows"

	"github.com/ayanacorp/proxymon/internal/app"
)

var (
	dashboardUser32 = windows.NewLazySystemDLL("user32.dll")

	procCallWindowProcW    = dashboardUser32.NewProc("CallWindowProcW")
	procDefWindowProcW     = dashboardUser32.NewProc("DefWindowProcW")
	procGetCursorPos       = dashboardUser32.NewProc("GetCursorPos")
	procGetSystemMetrics   = dashboardUser32.NewProc("GetSystemMetrics")
	procIsWindowVisible    = dashboardUser32.NewProc("IsWindowVisible")
	procMoveWindow         = dashboardUser32.NewProc("MoveWindow")
	procSetForeground      = dashboardUser32.NewProc("SetForegroundWindow")
	procSetWindowLongPtrW  = dashboardUser32.NewProc("SetWindowLongPtrW")
	procShowWindow         = dashboardUser32.NewProc("ShowWindow")

	webviewWndProc     = windows.NewCallback(webviewWindowProc)
	webviewWindowTable sync.Map
)

const (
	windowTitle = "Proxymon"

	defaultWindowWidth  = 1120
	defaultWindowHeight = 820
	minWindowWidth      = 820
	minWindowHeight     = 620

	wmClose = 0x0010

	gwlpWndProc = ^uintptr(3)

	swHide    = 0
	swShow    = 5
	swRestore = 9

	smCxScreen = 0
	smCyScreen = 1
)

type point struct {
	X int32
	Y int32
}

type Window struct {
	mu sync.Mutex

	controller *app.Controller
	view       webview.WebView
	hwnd       windows.Handle
	origWndProc uintptr
	closing    bool

	latest          app.Snapshot
	pendingPoint    point
	hasPendingPoint bool

	ready chan error
	done  chan struct{}
}

func New(controller *app.Controller) (*Window, error) {
	d := &Window{
		controller: controller,
		ready:      make(chan error, 1),
		done:       make(chan struct{}),
	}
	go d.run()
	if err := <-d.ready; err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Window) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(d.done)

	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--disable-gpu-shader-disk-cache "+
			"--disable-features=msSmartScreenProtection "+
			"--disk-cache-size=1 "+
			"--disable-background-networking")

	view := webview.New(false)
	if view == nil {
		d.ready <- fmt.Errorf("create webview window: unavailable")
		return
	}

	view.SetTitle(windowTitle)
	view.SetSize(minWindowWidth, minWindowHeight, webview.HintMin)
	view.SetSize(defaultWindowWidth, defaultWindowHeight, webview.HintNone)
	view.Init(bridgeInitJS)
	if err := view.Bind("resetStats", func() (bool, error) {
		if err := d.controller.ResetStats(); err != nil {
			return false, err
		}
		return true, nil
	}); err != nil {
		view.Destroy()
		d.ready <- err
		return
	}
	view.SetHtml(embeddedDocument())

	hwnd := windows.Handle(uintptr(view.Window()))
	if hwnd == 0 {
		view.Destroy()
		d.ready <- fmt.Errorf("obtain webview window handle: unavailable")
		return
	}

	oldProc, _, callErr := procSetWindowLongPtrW.Call(
		uintptr(hwnd),
		gwlpWndProc,
		webviewWndProc,
	)
	if oldProc == 0 {
		view.Destroy()
		d.ready <- callErr
		return
	}

	d.mu.Lock()
	d.view = view
	d.hwnd = hwnd
	d.origWndProc = oldProc
	d.mu.Unlock()
	webviewWindowTable.Store(hwnd, d)
	procShowWindow.Call(uintptr(hwnd), uintptr(swHide))

	d.ready <- nil
	view.Run()

	webviewWindowTable.Delete(hwnd)
	if d.origWndProc != 0 {
		procSetWindowLongPtrW.Call(uintptr(hwnd), gwlpWndProc, d.origWndProc)
	}
	view.Destroy()
}

func (d *Window) Show(snapshot app.Snapshot) {
	pt, ok := cursorPos()
	d.mu.Lock()
	d.latest = snapshot
	if ok {
		d.pendingPoint = pt
		d.hasPendingPoint = true
	}
	view := d.view
	d.mu.Unlock()
	if view == nil {
		return
	}

	view.Dispatch(func() {
		d.publishSnapshot(snapshot)
		d.showWindow()
	})
}

func (d *Window) Update(snapshot app.Snapshot) {
	d.mu.Lock()
	d.latest = snapshot
	hwnd := d.hwnd
	view := d.view
	d.mu.Unlock()
	if view == nil || !isVisible(hwnd) {
		return
	}

	view.Dispatch(func() {
		d.publishSnapshot(snapshot)
	})
}

func (d *Window) Close() {
	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		<-d.done
		return
	}
	d.closing = true
	view := d.view
	d.mu.Unlock()
	if view != nil {
		view.Dispatch(func() {
			view.Terminate()
		})
	}
	<-d.done
}

func (d *Window) publishSnapshot(snapshot app.Snapshot) {
	d.mu.Lock()
	d.latest = snapshot
	view := d.view
	d.mu.Unlock()
	if view == nil {
		return
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	view.SetTitle(fmt.Sprintf("%s — ↑ %s/s ↓ %s/s", windowTitle, formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))
	view.Eval("window.__receiveSnapshot(" + string(payload) + ");")
}

func (d *Window) showWindow() {
	if d.hwnd == 0 {
		return
	}
	if !isVisible(d.hwnd) {
		d.positionNearCursor()
		procShowWindow.Call(uintptr(d.hwnd), uintptr(swShow))
	} else {
		procShowWindow.Call(uintptr(d.hwnd), uintptr(swRestore))
	}
	procSetForeground.Call(uintptr(d.hwnd))
}

func (d *Window) positionNearCursor() {
	d.mu.Lock()
	pt := d.pendingPoint
	hasPoint := d.hasPendingPoint
	d.hasPendingPoint = false
	d.mu.Unlock()
	if !hasPoint || d.hwnd == 0 {
		return
	}

	screenW := metric(smCxScreen)
	screenH := metric(smCyScreen)
	x := int(pt.X) - defaultWindowWidth + 36
	y := int(pt.Y) - defaultWindowHeight - 18
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+defaultWindowWidth > screenW {
		x = screenW - defaultWindowWidth
	}
	if y+defaultWindowHeight > screenH {
		y = screenH - defaultWindowHeight
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	procMoveWindow.Call(uintptr(d.hwnd), uintptr(x), uintptr(y), uintptr(defaultWindowWidth), uintptr(defaultWindowHeight), 1)
}

func webviewWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == wmClose {
		if value, ok := webviewWindowTable.Load(windows.Handle(hwnd)); ok {
			window := value.(*Window)
			window.mu.Lock()
			closing := window.closing
			window.mu.Unlock()
			if !closing {
				procShowWindow.Call(hwnd, uintptr(swHide))
				return 0
			}
		}
	}
	if value, ok := webviewWindowTable.Load(windows.Handle(hwnd)); ok {
		window := value.(*Window)
		ret, _, _ := procCallWindowProcW.Call(window.origWndProc, hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func cursorPos() (point, bool) {
	var pt point
	res, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt, res != 0
}

func metric(metric int) int {
	res, _, _ := procGetSystemMetrics.Call(uintptr(metric))
	return int(res)
}

func isVisible(hwnd windows.Handle) bool {
	res, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return res != 0
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

const bridgeInitJS = `
window.__proxymonQueue = window.__proxymonQueue || [];
window.__receiveSnapshot = function(snapshot) {
  if (typeof window.pushSnapshot === 'function') {
    window.pushSnapshot(snapshot);
  } else {
    window.__proxymonQueue.push(snapshot);
  }
};
window.__dashboardReady = function() {
  if (typeof window.pushSnapshot !== 'function') {
    return;
  }
  while (window.__proxymonQueue.length) {
    window.pushSnapshot(window.__proxymonQueue.shift());
  }
};
`
