package tray

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ayanacorp/proxymon/internal/app"
)

var (
	dashboardUser32 = windows.NewLazySystemDLL("user32.dll")
	dashboardGDI32  = windows.NewLazySystemDLL("gdi32.dll")
	dashboardK32    = windows.NewLazySystemDLL("kernel32.dll")

	procDashboardCreateFontW      = dashboardGDI32.NewProc("CreateFontW")
	procDashboardDeleteObject     = dashboardGDI32.NewProc("DeleteObject")
	procDashboardGetModuleHandleW = dashboardK32.NewProc("GetModuleHandleW")

	procDashboardCreateWindowExW  = dashboardUser32.NewProc("CreateWindowExW")
	procDashboardDefWindowProcW   = dashboardUser32.NewProc("DefWindowProcW")
	procDashboardDestroyWindow    = dashboardUser32.NewProc("DestroyWindow")
	procDashboardDispatchMessageW = dashboardUser32.NewProc("DispatchMessageW")
	procDashboardGetClientRect    = dashboardUser32.NewProc("GetClientRect")
	procDashboardGetCursorPos     = dashboardUser32.NewProc("GetCursorPos")
	procDashboardGetMessageW      = dashboardUser32.NewProc("GetMessageW")
	procDashboardGetSystemMetrics = dashboardUser32.NewProc("GetSystemMetrics")
	procDashboardIsWindowVisible  = dashboardUser32.NewProc("IsWindowVisible")
	procDashboardLoadCursorW      = dashboardUser32.NewProc("LoadCursorW")
	procDashboardMoveWindow       = dashboardUser32.NewProc("MoveWindow")
	procDashboardPostMessageW     = dashboardUser32.NewProc("PostMessageW")
	procDashboardPostQuitMessage  = dashboardUser32.NewProc("PostQuitMessage")
	procDashboardRegisterClassExW = dashboardUser32.NewProc("RegisterClassExW")
	procDashboardSendMessageW     = dashboardUser32.NewProc("SendMessageW")
	procDashboardSetForeground    = dashboardUser32.NewProc("SetForegroundWindow")
	procDashboardSetWindowTextW   = dashboardUser32.NewProc("SetWindowTextW")
	procDashboardShowWindow       = dashboardUser32.NewProc("ShowWindow")
	procDashboardTranslateMessage = dashboardUser32.NewProc("TranslateMessage")
	procDashboardUnregisterClassW = dashboardUser32.NewProc("UnregisterClassW")
	procDashboardUpdateWindow     = dashboardUser32.NewProc("UpdateWindow")
)

const (
	dashboardClassName = "ProxymonDashboardWindow"

	dashboardDefaultWindowWidth  = 660
	dashboardDefaultWindowHeight = 560
	dashboardMinWindowWidth      = 560
	dashboardMinWindowHeight     = 440

	dashboardOuterPadding    = 18
	dashboardSectionGap      = 14
	dashboardGroupPadding    = 16
	dashboardHeaderRowHeight = 30
	dashboardStatusHeight    = 22
	dashboardFooterHeight    = 24
	dashboardOverviewHeight  = 176
	dashboardRowHeight       = 26
	dashboardLabelWidth      = 106
	dashboardFieldGap        = 8
	dashboardColumnGap       = 28
	dashboardGroupTopOffset  = 28

	wmDashboardUpdate   = 0x8001
	wmDashboardShow     = 0x8002
	wmDashboardShutdown = 0x8003
	wmSetFont           = 0x0030
	wmSize              = 0x0005
	wmClose             = 0x0010
	wmDestroy           = 0x0002
	wmGetMinMaxInfo     = 0x0024

	colorWindowPlusOne = 6
	idcArrow           = 32512

	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsThickFrame  = 0x00040000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	wsVScroll     = 0x00200000

	wsExClientEdge = 0x00000200
	wsExToolWindow = 0x00000080

	esMultiline   = 0x0004
	esAutoVScroll = 0x0040
	esReadOnly    = 0x0800

	ssRight    = 0x00000002
	bsGroupBox = 0x00000007

	swHide    = 0
	swShow    = 5
	swRestore = 9

	smCxScreen = 0
	smCyScreen = 1

	fwNormal = 400
	fwBold   = 700
)

type dashboardPoint struct {
	X int32
	Y int32
}

type dashboardRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type dashboardMsg struct {
	windowHandle windows.Handle
	message      uint32
	wParam       uintptr
	lParam       uintptr
	time         uint32
	pt           dashboardPoint
}

type dashboardWndClassEx struct {
	size, style                        uint32
	wndProc                            uintptr
	clsExtra, wndExtra                 int32
	instance, icon, cursor, background windows.Handle
	menuName, className                *uint16
	iconSm                             windows.Handle
}

type dashboardMinMaxInfo struct {
	reserved     dashboardPoint
	maxSize      dashboardPoint
	maxPosition  dashboardPoint
	minTrackSize dashboardPoint
	maxTrackSize dashboardPoint
}

type dashboardStatField struct {
	label windows.Handle
	value windows.Handle
}

type dashboardWindow struct {
	mu sync.Mutex

	hwnd       windows.Handle
	instance   windows.Handle
	className  *uint16
	title      windows.Handle
	speed      windows.Handle
	status     windows.Handle
	footer     windows.Handle
	overview   windows.Handle
	interfaces windows.Handle

	modeField        dashboardStatField
	httpField        dashboardStatField
	socksField       dashboardStatField
	autoProxyField   dashboardStatField
	ifaceCountField  dashboardStatField
	connectionsField dashboardStatField
	uploadField      dashboardStatField
	downloadField    dashboardStatField
	interfacesBox    windows.Handle

	titleFont   windows.Handle
	speedFont   windows.Handle
	sectionFont windows.Handle
	labelFont   windows.Handle
	valueFont   windows.Handle
	monoFont    windows.Handle

	latest          app.Snapshot
	pendingPoint    dashboardPoint
	hasPendingPoint bool
	ready           chan error
	done            chan struct{}
}

func newDashboardWindow() (*dashboardWindow, error) {
	d := &dashboardWindow{
		ready: make(chan error, 1),
		done:  make(chan struct{}),
	}
	go d.run()
	if err := <-d.ready; err != nil {
		return nil, err
	}
	return d, nil
}

func (d *dashboardWindow) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(d.done)

	if err := d.init(); err != nil {
		d.ready <- err
		return
	}
	d.ready <- nil

	msg := &dashboardMsg{}
	for {
		ret, _, err := procDashboardGetMessageW.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			_ = err
			return
		case 0:
			return
		default:
			procDashboardTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
			procDashboardDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
		}
	}
}

func (d *dashboardWindow) init() error {
	instanceHandle, _, err := procDashboardGetModuleHandleW.Call(0)
	if instanceHandle == 0 {
		return err
	}
	d.instance = windows.Handle(instanceHandle)

	classNamePtr, err := windows.UTF16PtrFromString(dashboardClassName)
	if err != nil {
		return err
	}
	d.className = classNamePtr

	cursorHandle, _, err := procDashboardLoadCursorW.Call(0, uintptr(idcArrow))
	if cursorHandle == 0 {
		return err
	}

	wcex := &dashboardWndClassEx{
		style:      0x0002 | 0x0001,
		wndProc:    windows.NewCallback(d.wndProc),
		instance:   d.instance,
		cursor:     windows.Handle(cursorHandle),
		background: windows.Handle(colorWindowPlusOne),
		className:  classNamePtr,
	}
	wcex.size = uint32(unsafe.Sizeof(*wcex))

	res, _, err := procDashboardRegisterClassExW.Call(uintptr(unsafe.Pointer(wcex)))
	if res == 0 {
		return err
	}

	titlePtr, err := windows.UTF16PtrFromString("Proxymon")
	if err != nil {
		return err
	}

	hwnd, _, err := procDashboardCreateWindowExW.Call(
		uintptr(wsExToolWindow),
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(wsCaption|wsSysMenu|wsMinimizeBox|wsThickFrame),
		uintptr(180),
		uintptr(140),
		uintptr(dashboardDefaultWindowWidth),
		uintptr(dashboardDefaultWindowHeight),
		0,
		0,
		uintptr(d.instance),
		0,
	)
	if hwnd == 0 {
		procDashboardUnregisterClassW.Call(uintptr(unsafe.Pointer(classNamePtr)), uintptr(d.instance))
		return err
	}
	d.hwnd = windows.Handle(hwnd)

	if err := d.createFonts(); err != nil {
		return err
	}
	if err := d.createControls(); err != nil {
		return err
	}
	d.layoutControls()

	procDashboardShowWindow.Call(uintptr(d.hwnd), uintptr(swHide))
	procDashboardUpdateWindow.Call(uintptr(d.hwnd))
	return nil
}

func (d *dashboardWindow) createFonts() error {
	var err error
	if d.titleFont, err = createDashboardFont("Segoe UI Semibold", -26, fwBold); err != nil {
		return err
	}
	if d.speedFont, err = createDashboardFont("Segoe UI Semibold", -22, fwBold); err != nil {
		return err
	}
	if d.sectionFont, err = createDashboardFont("Segoe UI Semibold", -16, fwBold); err != nil {
		return err
	}
	if d.labelFont, err = createDashboardFont("Segoe UI", -15, fwNormal); err != nil {
		return err
	}
	if d.valueFont, err = createDashboardFont("Segoe UI Semibold", -15, fwBold); err != nil {
		return err
	}
	if d.monoFont, err = createDashboardFont("Consolas", -15, fwNormal); err != nil {
		return err
	}
	return nil
}

func (d *dashboardWindow) createControls() error {
	var err error

	if d.title, err = createDashboardStatic(d.hwnd, "Proxymon", 0); err != nil {
		return err
	}
	if d.speed, err = createDashboardStatic(d.hwnd, "↑ 0 B/s  ↓ 0 B/s", ssRight); err != nil {
		return err
	}
	if d.status, err = createDashboardStatic(d.hwnd, "Live proxy status refreshed every second.", 0); err != nil {
		return err
	}
	if d.overview, err = createDashboardGroup(d.hwnd, "Overview"); err != nil {
		return err
	}
	if d.interfaces, err = createDashboardGroup(d.hwnd, "Selected Interfaces"); err != nil {
		return err
	}
	if d.footer, err = createDashboardStatic(d.hwnd, "Left-click tray icon to reopen this dashboard.", 0); err != nil {
		return err
	}

	if d.modeField, err = createDashboardField(d.hwnd, "Mode"); err != nil {
		return err
	}
	if d.httpField, err = createDashboardField(d.hwnd, "HTTP Proxy"); err != nil {
		return err
	}
	if d.socksField, err = createDashboardField(d.hwnd, "SOCKS5"); err != nil {
		return err
	}
	if d.autoProxyField, err = createDashboardField(d.hwnd, "Auto Proxy"); err != nil {
		return err
	}
	if d.ifaceCountField, err = createDashboardField(d.hwnd, "Interfaces"); err != nil {
		return err
	}
	if d.connectionsField, err = createDashboardField(d.hwnd, "Connections"); err != nil {
		return err
	}
	if d.uploadField, err = createDashboardField(d.hwnd, "Total Upload"); err != nil {
		return err
	}
	if d.downloadField, err = createDashboardField(d.hwnd, "Total Download"); err != nil {
		return err
	}
	if d.interfacesBox, err = createDashboardEdit(d.hwnd); err != nil {
		return err
	}

	setDashboardFont(d.title, d.titleFont)
	setDashboardFont(d.speed, d.speedFont)
	setDashboardFont(d.status, d.labelFont)
	setDashboardFont(d.overview, d.sectionFont)
	setDashboardFont(d.interfaces, d.sectionFont)
	setDashboardFont(d.footer, d.labelFont)
	for _, field := range d.fields() {
		setDashboardFont(field.label, d.labelFont)
		setDashboardFont(field.value, d.valueFont)
	}
	setDashboardFont(d.interfacesBox, d.monoFont)

	return nil
}

func (d *dashboardWindow) Show(snapshot app.Snapshot) {
	pt, ok := dashboardCursorPos()
	d.mu.Lock()
	d.latest = snapshot
	if ok {
		d.pendingPoint = pt
		d.hasPendingPoint = true
	}
	hwnd := d.hwnd
	d.mu.Unlock()
	if hwnd != 0 {
		procDashboardPostMessageW.Call(uintptr(hwnd), uintptr(wmDashboardShow), 0, 0)
	}
}

func (d *dashboardWindow) Update(snapshot app.Snapshot) {
	d.mu.Lock()
	d.latest = snapshot
	hwnd := d.hwnd
	d.mu.Unlock()
	if hwnd != 0 {
		procDashboardPostMessageW.Call(uintptr(hwnd), uintptr(wmDashboardUpdate), 0, 0)
	}
}

func (d *dashboardWindow) Close() {
	d.mu.Lock()
	hwnd := d.hwnd
	d.mu.Unlock()
	if hwnd != 0 {
		procDashboardPostMessageW.Call(uintptr(hwnd), uintptr(wmDashboardShutdown), 0, 0)
	}
	<-d.done
}

func (d *dashboardWindow) wndProc(hWnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmDashboardUpdate:
		d.applySnapshot()
		return 0
	case wmDashboardShow:
		d.showWindow()
		return 0
	case wmDashboardShutdown:
		procDashboardDestroyWindow.Call(uintptr(hWnd))
		return 0
	case wmSize:
		d.layoutControls()
		return 0
	case wmClose:
		procDashboardShowWindow.Call(uintptr(hWnd), uintptr(swHide))
		return 0
	case wmDestroy:
		d.cleanupFonts()
		procDashboardUnregisterClassW.Call(uintptr(unsafe.Pointer(d.className)), uintptr(d.instance))
		procDashboardPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDashboardDefWindowProcW.Call(
		uintptr(hWnd),
		uintptr(message),
		uintptr(wParam),
		uintptr(lParam),
	)
	return ret
}

func (d *dashboardWindow) showWindow() {
	d.applySnapshot()
	if !dashboardIsVisible(d.hwnd) {
		d.positionNearCursor()
		procDashboardShowWindow.Call(uintptr(d.hwnd), uintptr(swShow))
	} else {
		procDashboardShowWindow.Call(uintptr(d.hwnd), uintptr(swRestore))
	}
	procDashboardSetForeground.Call(uintptr(d.hwnd))
}

func (d *dashboardWindow) applySnapshot() {
	d.mu.Lock()
	snapshot := d.latest
	d.mu.Unlock()

	setDashboardText(d.title, "Proxymon")
	setDashboardText(d.speed, fmt.Sprintf("↑ %s/s    ↓ %s/s", formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))
	setDashboardText(d.status, dashboardStatusText(snapshot))
	setDashboardText(d.modeField.value, snapshot.Mode.String())
	setDashboardText(d.httpField.value, nonEmpty(snapshot.ProxyAddr, "—"))
	setDashboardText(d.socksField.value, nonEmpty(snapshot.Socks5Addr, "—"))
	setDashboardText(d.autoProxyField.value, boolLabel(snapshot.WinProxyAuto))
	setDashboardText(d.ifaceCountField.value, fmt.Sprintf("%d active", snapshot.ActiveInterfaces))
	setDashboardText(d.connectionsField.value, fmt.Sprintf("%d active / %d total", snapshot.ActiveConnections, snapshot.TotalConnections))
	setDashboardText(d.uploadField.value, formatBytes(snapshot.BytesSent))
	setDashboardText(d.downloadField.value, formatBytes(snapshot.BytesRecv))
	setDashboardText(d.interfacesBox, dashboardInterfacesText(snapshot))
	setDashboardText(d.footer, dashboardFooterText(snapshot))
	setDashboardText(d.hwnd, fmt.Sprintf("Proxymon — ↑ %s/s ↓ %s/s", formatBytes(snapshot.RateSent), formatBytes(snapshot.RateRecv)))
}

func (d *dashboardWindow) layoutControls() {
	width, height := d.clientSize()
	if width <= 0 || height <= 0 {
		return
	}

	contentX := dashboardOuterPadding
	contentW := width - (dashboardOuterPadding * 2)
	y := dashboardOuterPadding

	titleW := int(float64(contentW) * 0.46)
	if titleW < 240 {
		titleW = 240
	}
	speedW := contentW - titleW - dashboardSectionGap
	if speedW < 170 {
		speedW = 170
		titleW = contentW - speedW - dashboardSectionGap
	}

	moveDashboardWindow(d.title, contentX, y, titleW, dashboardHeaderRowHeight)
	moveDashboardWindow(d.speed, contentX+titleW+dashboardSectionGap, y, speedW, dashboardHeaderRowHeight)
	y += dashboardHeaderRowHeight + 4
	moveDashboardWindow(d.status, contentX, y, contentW, dashboardStatusHeight)
	y += dashboardStatusHeight + dashboardSectionGap

	footerY := height - dashboardOuterPadding - dashboardFooterHeight
	overviewY := y
	interfacesY := overviewY + dashboardOverviewHeight + dashboardSectionGap
	interfacesH := footerY - dashboardSectionGap - interfacesY
	if interfacesH < 140 {
		interfacesH = 140
	}

	moveDashboardWindow(d.overview, contentX, overviewY, contentW, dashboardOverviewHeight)
	d.layoutOverviewFields(contentX, overviewY, contentW)

	moveDashboardWindow(d.interfaces, contentX, interfacesY, contentW, interfacesH)
	innerX := contentX + dashboardGroupPadding
	innerY := interfacesY + dashboardGroupTopOffset
	innerW := contentW - (dashboardGroupPadding * 2)
	innerH := interfacesH - dashboardGroupTopOffset - dashboardGroupPadding
	if innerH < 60 {
		innerH = 60
	}
	moveDashboardWindow(d.interfacesBox, innerX, innerY, innerW, innerH)

	moveDashboardWindow(d.footer, contentX, footerY, contentW, dashboardFooterHeight)
}

func (d *dashboardWindow) layoutOverviewFields(groupX, groupY, groupW int) {
	innerX := groupX + dashboardGroupPadding
	innerY := groupY + dashboardGroupTopOffset
	innerW := groupW - (dashboardGroupPadding * 2)
	colW := (innerW - dashboardColumnGap) / 2
	leftX := innerX
	rightX := innerX + colW + dashboardColumnGap

	leftFields := []dashboardStatField{d.modeField, d.autoProxyField, d.ifaceCountField, d.connectionsField}
	rightFields := []dashboardStatField{d.httpField, d.socksField, d.uploadField, d.downloadField}

	for i, field := range leftFields {
		d.layoutField(field, leftX, innerY+(i*dashboardRowHeight), colW)
	}
	for i, field := range rightFields {
		d.layoutField(field, rightX, innerY+(i*dashboardRowHeight), colW)
	}
}

func (d *dashboardWindow) layoutField(field dashboardStatField, x, y, width int) {
	valueX := x + dashboardLabelWidth + dashboardFieldGap
	valueW := width - dashboardLabelWidth - dashboardFieldGap
	if valueW < 60 {
		valueW = 60
	}
	moveDashboardWindow(field.label, x, y, dashboardLabelWidth, dashboardRowHeight)
	moveDashboardWindow(field.value, valueX, y, valueW, dashboardRowHeight)
}

func (d *dashboardWindow) clientSize() (int, int) {
	var rect dashboardRect
	res, _, _ := procDashboardGetClientRect.Call(uintptr(d.hwnd), uintptr(unsafe.Pointer(&rect)))
	if res == 0 {
		return 0, 0
	}
	return int(rect.Right - rect.Left), int(rect.Bottom - rect.Top)
}

func (d *dashboardWindow) positionNearCursor() {
	d.mu.Lock()
	pt := d.pendingPoint
	hasPoint := d.hasPendingPoint
	d.hasPendingPoint = false
	d.mu.Unlock()
	if !hasPoint {
		return
	}

	screenW := dashboardMetric(smCxScreen)
	screenH := dashboardMetric(smCyScreen)
	x := int(pt.X) - dashboardDefaultWindowWidth + 36
	y := int(pt.Y) - dashboardDefaultWindowHeight - 18
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+dashboardDefaultWindowWidth > screenW {
		x = screenW - dashboardDefaultWindowWidth
	}
	if y+dashboardDefaultWindowHeight > screenH {
		y = screenH - dashboardDefaultWindowHeight
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	moveDashboardWindow(d.hwnd, x, y, dashboardDefaultWindowWidth, dashboardDefaultWindowHeight)
}

func (d *dashboardWindow) cleanupFonts() {
	for _, font := range []windows.Handle{d.titleFont, d.speedFont, d.sectionFont, d.labelFont, d.valueFont, d.monoFont} {
		if font != 0 {
			procDashboardDeleteObject.Call(uintptr(font))
		}
	}
}

func (d *dashboardWindow) fields() []dashboardStatField {
	return []dashboardStatField{
		d.modeField,
		d.httpField,
		d.socksField,
		d.autoProxyField,
		d.ifaceCountField,
		d.connectionsField,
		d.uploadField,
		d.downloadField,
	}
}

func createDashboardFont(face string, height int32, weight int32) (windows.Handle, error) {
	facePtr, err := windows.UTF16PtrFromString(face)
	if err != nil {
		return 0, err
	}
	res, _, callErr := procDashboardCreateFontW.Call(
		uintptr(height),
		0,
		0,
		0,
		uintptr(weight),
		0,
		0,
		0,
		1,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(facePtr)),
	)
	if res == 0 {
		return 0, callErr
	}
	return windows.Handle(res), nil
}

func createDashboardGroup(parent windows.Handle, title string) (windows.Handle, error) {
	return createDashboardChild(parent, "BUTTON", title, bsGroupBox)
}

func createDashboardStatic(parent windows.Handle, text string, style uint32) (windows.Handle, error) {
	return createDashboardChild(parent, "STATIC", text, style)
}

func createDashboardField(parent windows.Handle, label string) (dashboardStatField, error) {
	labelHandle, err := createDashboardStatic(parent, label+":", ssRight)
	if err != nil {
		return dashboardStatField{}, err
	}
	valueHandle, err := createDashboardStatic(parent, "—", 0)
	if err != nil {
		return dashboardStatField{}, err
	}
	return dashboardStatField{label: labelHandle, value: valueHandle}, nil
}

func createDashboardEdit(parent windows.Handle) (windows.Handle, error) {
	style := uint32(wsVScroll | esMultiline | esAutoVScroll | esReadOnly)
	return createDashboardChild(parent, "EDIT", "", style, wsExClientEdge)
}

func createDashboardChild(parent windows.Handle, className, text string, style uint32, exStyle ...uint32) (windows.Handle, error) {
	classPtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, err
	}
	var extended uint32
	if len(exStyle) > 0 {
		extended = exStyle[0]
	}
	res, _, callErr := procDashboardCreateWindowExW.Call(
		uintptr(extended),
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style|wsChild|wsVisible),
		0,
		0,
		0,
		0,
		uintptr(parent),
		0,
		0,
		0,
	)
	if res == 0 {
		return 0, callErr
	}
	return windows.Handle(res), nil
}

func setDashboardFont(hwnd windows.Handle, font windows.Handle) {
	procDashboardSendMessageW.Call(uintptr(hwnd), uintptr(wmSetFont), uintptr(font), 1)
}

func setDashboardText(hwnd windows.Handle, text string) {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procDashboardSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(textPtr)))
}

func moveDashboardWindow(hwnd windows.Handle, x, y, width, height int) {
	procDashboardMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
}

func dashboardMetric(metric int) int {
	res, _, _ := procDashboardGetSystemMetrics.Call(uintptr(metric))
	return int(res)
}

func dashboardCursorPos() (dashboardPoint, bool) {
	var pt dashboardPoint
	res, _, _ := procDashboardGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt, res != 0
}

func dashboardIsVisible(hwnd windows.Handle) bool {
	res, _, _ := procDashboardIsWindowVisible.Call(uintptr(hwnd))
	return res != 0
}

func dashboardStatusText(snapshot app.Snapshot) string {
	if snapshot.LastError != "" {
		return "Attention needed — a recent runtime issue was recorded."
	}
	return fmt.Sprintf("Proxy running • %d active interface(s) • refreshed every second", snapshot.ActiveInterfaces)
}

func dashboardInterfacesText(snapshot app.Snapshot) string {
	var b strings.Builder
	b.WriteString("NAME               IP              ST   UPLOAD       DOWNLOAD     GATEWAY\r\n")
	b.WriteString("--------------------------------------------------------------------------\r\n")

	rows := 0
	for _, iface := range snapshot.Interfaces {
		if !iface.Selected {
			continue
		}
		rows++
		status := "DOWN"
		if iface.Alive {
			status = "UP"
		}
		name := iface.FriendlyName
		if name == "" {
			name = iface.Name
		}
		b.WriteString(fmt.Sprintf("%-18s %-15s %-4s %-12s %-12s %-16s\r\n",
			fitDashboardText(name, 18),
			fitDashboardText(iface.IP, 15),
			status,
			fitDashboardText(formatBytes(iface.BytesSent), 12),
			fitDashboardText(formatBytes(iface.BytesRecv), 12),
			fitDashboardText(iface.Gateway, 16),
		))
	}
	if rows == 0 {
		b.WriteString("No selected interfaces.\r\n")
	}
	return b.String()
}

func dashboardFooterText(snapshot app.Snapshot) string {
	if snapshot.LastError != "" {
		return fitDashboardText("Last error: "+snapshot.LastError, 120)
	}
	return "Left-click tray icon to focus this dashboard. Right-click for quick actions."
}

func fitDashboardText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func boolLabel(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
