//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Size of the reminder card in CSS pixels; scaled to the screen's DPI.
const (
	popupWidth  = 460
	popupHeight = 228
	popupClass  = "MeetingReminderPopup"
)

const (
	wsPopup                     = 0x80000000
	wsExTopmost                 = 0x00000008
	wsExAppWindow               = 0x00040000
	wmDestroy                   = 0x0002
	wmSize                      = 0x0005
	wmActivate                  = 0x0006
	wmClose                     = 0x0010
	swShow                      = 5
	spiGetWorkArea              = 0x0030
	imageIcon                   = 1
	lrDefaultSize               = 0x00000040
	lrShared                    = 0x00008000
	idcArrow                    = 32512
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
)

var (
	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procDwmSetWindowAttr     = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

	registerClass = sync.OnceValue(registerPopupClass)
	// popups maps a popup window handle to its browser, for WM_SIZE.
	popups sync.Map
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

type winRect struct{ left, top, right, bottom int32 }

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
		if c, ok := popups.Load(hwnd); ok {
			c.(*edge.Chromium).Resize()
		}
	case wmActivate:
		if c, ok := popups.Load(hwnd); ok && wParam != 0 {
			c.(*edge.Chromium).Focus()
		}
	case wmDestroy:
		popups.Delete(hwnd)
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// showPopup shows the reminder card for e and blocks until the user closes
// it, returning the chosen action. Each card gets its own OS thread, since a
// window and its WebView2 belong to the thread that created them.
func showPopup(e Event) (string, error) {
	page, err := popupHTML(e, time.Now())
	if err != nil {
		return "", err
	}
	type result struct {
		action string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		// Never unlocked: the thread exits with this goroutine, which also
		// releases its COM apartment and window.
		runtime.LockOSThread()
		action, err := runPopup(page)
		done <- result{action, err}
	}()
	r := <-done
	return r.action, r.err
}

func runPopup(page string) (string, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(1)) { // 1 = S_FALSE
		return "", fmt.Errorf("CoInitializeEx: %w", err)
	}
	if err := registerClass(); err != nil {
		return "", err
	}

	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	w := int32(popupWidth * dpi / 96)
	h := int32(popupHeight * dpi / 96)
	var work winRect
	_, _, _ = procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)
	x := work.left + (work.right-work.left-w)/2
	y := work.top + (work.bottom-work.top-h)/3

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
	_, _, _ = procDwmSetWindowAttr.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))

	action := actionDismiss
	browser := edge.NewChromium()
	browser.DataPath = webviewDataDir()
	browser.MessageCallback = func(msg string) {
		switch msg {
		case actionJoin, actionSnooze, actionDismiss:
			action = msg
			_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
	}
	shown := false
	browser.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		// Show the window only once the page has rendered, to avoid a white flash.
		if !shown {
			shown = true
			_, _, _ = procShowWindow.Call(hwnd, swShow)
			_, _, _ = procSetForegroundWindow.Call(hwnd)
			browser.Focus()
		}
	}
	popups.Store(hwnd, browser)

	if !browser.Embed(hwnd) {
		_, _, _ = procDestroyWindow.Call(hwnd)
		return "", errors.New("WebView2 runtime is not available")
	}
	if settings, err := browser.GetSettings(); err == nil {
		_ = settings.PutAreDefaultContextMenusEnabled(false)
		_ = settings.PutAreDevToolsEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutIsStatusBarEnabled(false)
		_ = settings.PutAreBrowserAcceleratorKeysEnabled(false)
	}
	browser.Resize()
	browser.NavigateToString(page)

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { //nolint:gosec // G115: GetMessage returns a BOOL; 0 is WM_QUIT, -1 an error
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
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

// openURL opens an https link or a local file with its default handler.
func openURL(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
