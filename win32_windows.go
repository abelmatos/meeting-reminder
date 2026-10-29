//go:build windows

package main

import "golang.org/x/sys/windows"

// Win32 DLLs, procedures and constants used by the app. Only system DLLs are
// loaded (from System32).

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
)

// Clipboard (platform_windows.go).
var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

// Beep (reminder_windows.go).
var procMessageBeep = user32.NewProc("MessageBeep")

// Card window (cardwindow_windows.go).
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
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
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
