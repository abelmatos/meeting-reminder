//go:build windows

package main

import (
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	beepInterval = 30 * time.Second
	appTitle     = "Meeting Reminder"

	mbOK              = 0x00000000
	mbOKCancel        = 0x00000001
	mbIconError       = 0x00000010
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbSystemModal     = 0x00001000
	mbSetForeground   = 0x00010000
	mbTopmost         = 0x00040000
	idOK              = 1
)

var procMessageBeep = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBeep")

// alert shows a topmost popup for e and beeps every beepInterval until the
// user clicks OK.
func alert(e Event) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(beepInterval)
		defer t.Stop()
		for {
			_, _, _ = procMessageBeep.Call(mbIconWarning)
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()
	messageBox(formatAlert(e, time.Now()), mbOK|mbIconWarning|mbSystemModal|mbSetForeground|mbTopmost)
	close(done)
}

func showInfo(msg string) {
	messageBox(msg, mbOK|mbIconInformation|mbSetForeground|mbTopmost)
}

func showError(msg string) {
	messageBox(msg, mbOK|mbIconError|mbSetForeground|mbTopmost)
}

// confirm shows an OK/Cancel box and reports whether OK was clicked.
func confirm(msg string) bool {
	return messageBox(msg, mbOKCancel|mbIconInformation|mbSetForeground|mbTopmost) == idOK
}

// messageBox shows a blocking MessageBoxW and returns the clicked button.
func messageBox(text string, flags uint32) int32 {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t, _ := windows.UTF16PtrFromString(strings.ReplaceAll(text, "\x00", ""))
	c, _ := windows.UTF16PtrFromString(appTitle)
	r, _ := windows.MessageBox(0, t, c, flags)
	return r
}
