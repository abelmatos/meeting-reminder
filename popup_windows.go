//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Size of the reminder card in CSS pixels; scaled to the monitor's DPI.
const (
	popupWidth  = 460
	popupHeight = 228
	popupClass  = "MeetingReminderPopup"
)

const (
	wsPopup                 = 0x80000000
	wsExTopmost             = 0x00000008
	wsExAppWindow           = 0x00040000
	wmDestroy               = 0x0002
	wmSize                  = 0x0005
	wmActivate              = 0x0006
	wmSetFocus              = 0x0007
	wmClose                 = 0x0010
	wmDpiChanged            = 0x02E0
	waInactive              = 0
	imageIcon               = 1
	lrDefaultSize           = 0x00000040
	lrShared                = 0x00008000
	idcArrow                = 32512
	monitorDefaultToNearest = 2
	mdtEffectiveDpi         = 0
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
	dwmwcpRound             = 2
)

var (
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procGetDpiForMonitor    = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")

	// card is the browser of this process's only window (see card_windows.go).
	card *edge.Chromium
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

type monitorInfo struct {
	size    uint32
	monitor windows.Rect
	work    windows.Rect
	flags   uint32
}

func registerPopupClass() error {
	var inst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &inst); err != nil {
		return err
	}
	appIcon, _ := windows.UTF16PtrFromString("APP") // icon from winres/winres.json
	icon, _, _ := procLoadImageW.Call(uintptr(inst), uintptr(unsafe.Pointer(appIcon)), imageIcon, 0, 0, lrDefaultSize|lrShared)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	className, _ := windows.UTF16PtrFromString(popupClass)
	wc := wndClassEx{
		wndProc:   windows.NewCallback(popupWndProc),
		instance:  inst,
		icon:      windows.Handle(icon),
		cursor:    windows.Handle(cursor),
		className: className,
		iconSm:    windows.Handle(icon),
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	return nil
}

func popupWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmSize:
		if card != nil {
			card.Resize()
		}
	case wmActivate:
		// Keep keyboard focus in the page (Enter/Esc); DefWindowProc would
		// move it to the bare window.
		if card != nil && wParam&0xFFFF != waInactive {
			card.Focus()
			return 0
		}
	case wmSetFocus:
		if card != nil {
			card.Focus()
			return 0
		}
	case wmDpiChanged:
		// Moved to a monitor with another scale: take the suggested rect.
		// lParam points to a RECT; reinterpret it without a uintptr-to-pointer
		// conversion, which go vet rejects.
		r := *(**windows.Rect)(unsafe.Pointer(&lParam))
		x, y, w, h := uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top) //nolint:gosec // G115: Win32 reads these as int; negative x/y are valid
		_, _, _ = procSetWindowPos.Call(hwnd, 0, x, y, w, h, swpNoZOrder|swpNoActivate)
		return 0
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// cardPlacement centres the card on the monitor under the mouse cursor,
// sized for that monitor's DPI.
func cardPlacement() (x, y, w, h int32) {
	var pt struct{ x, y int32 }
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// POINT is passed by value, packed into one register on amd64.
	packed := uintptr(uint32(pt.x)) | uintptr(uint32(pt.y))<<32 //nolint:gosec // G115: bit-packing two int32s
	mon, _, _ := procMonitorFromPoint.Call(packed, monitorDefaultToNearest)
	mi := monitorInfo{}
	mi.size = uint32(unsafe.Sizeof(mi))
	_, _, _ = procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	dpiX, dpiY := uint32(96), uint32(96)
	if hr, _, _ := procGetDpiForMonitor.Call(mon, mdtEffectiveDpi, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); hr != 0 {
		dpiX = 96
	}
	w = int32(popupWidth * dpiX / 96)  //nolint:gosec // G115: small positive values
	h = int32(popupHeight * dpiX / 96) //nolint:gosec // G115: small positive values
	work := mi.work
	x = work.Left + (work.Right-work.Left-w)/2
	y = work.Top + (work.Bottom-work.Top-h)/3
	return x, y, w, h
}

// runPopup shows page in a borderless, topmost WebView2 window and blocks
// until the page posts an action or the window closes. onShown runs once the
// page has rendered. It must run on a thread with a COM apartment, and the
// process may host only one card.
func runPopup(page, variant string, onShown func()) (string, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(1)) { // 1 = S_FALSE
		return "", fmt.Errorf("CoInitializeEx: %w", err)
	}
	if err := registerPopupClass(); err != nil {
		return "", err
	}

	x, y, w, h := cardPlacement()
	className, _ := windows.UTF16PtrFromString(popupClass)
	title, _ := windows.UTF16PtrFromString(appTitle)
	hwnd, _, err := procCreateWindowExW.Call(
		wsExTopmost|wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), //nolint:gosec // G115: Win32 reads these as int; negative x/y are valid
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return "", fmt.Errorf("CreateWindowEx: %w", err)
	}
	// Windows 11 rounds the corners and draws a border and shadow; older
	// versions ignore this.
	corner := uint32(dwmwcpRound)
	_ = windows.DwmSetWindowAttribute(windows.HWND(hwnd), windows.DWMWA_WINDOW_CORNER_PREFERENCE, unsafe.Pointer(&corner), uint32(unsafe.Sizeof(corner)))

	action := ""
	card = edge.NewChromium()
	card.DataPath = webviewDataDir()
	card.MessageCallback = func(msg string) {
		// Keep only the first action; the window is already closing.
		if action != "" {
			return
		}
		switch msg {
		case actionJoin, actionSnooze, actionDismiss:
			action = msg
			_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
	}
	shown := false
	card.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		// Show the window only once the page has rendered, to avoid a white flash.
		if !shown {
			shown = true
			_, _, _ = procShowWindow.Call(hwnd, windows.SW_SHOW)
			_, _, _ = procSetForegroundWindow.Call(hwnd)
			if variant == "refresh" {
				if c := card.GetController(); c != nil {
					_ = c.PutIsVisible(false)
					_ = c.PutIsVisible(true)
				}
				_ = card.NotifyParentWindowPositionChanged()
				card.Resize()
			}
			card.Focus()
			log.Print("shown")
			onShown()
		}
	}

	log.Printf("showing %dx%d at %d,%d", w, h, x, y)
	if !card.Embed(hwnd) {
		_, _, _ = procDestroyWindow.Call(hwnd)
		return "", errors.New("WebView2 runtime is not available")
	}
	if settings, err := card.GetSettings(); err == nil {
		_ = settings.PutAreDefaultContextMenusEnabled(false)
		_ = settings.PutAreDevToolsEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutIsStatusBarEnabled(false)
		_ = settings.PutAreBrowserAcceleratorKeysEnabled(false)
	}
	card.Resize()
	if variant == "show-first" {
		_, _, _ = procShowWindow.Call(hwnd, windows.SW_SHOW)
	}
	card.NavigateToString(page)

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { //nolint:gosec // G115: GetMessage returns a BOOL; 0 is WM_QUIT, -1 an error
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	if action == "" {
		action = actionDismiss // closed some other way, e.g. Alt+F4
	}
	return action, nil
}

// webviewDataDir keeps WebView2's profile out of the exe's folder.
func webviewDataDir() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "meeting-reminder", "WebView2")
}
