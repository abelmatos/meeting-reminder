//go:build windows

package main

import (
	"log"
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

// Actions the reminder popup can return; popup.html posts them.
const (
	actionJoin    = "join"
	actionSnooze  = "snooze"
	actionDismiss = "dismiss"
)

var procMessageBeep = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBeep")

// alert shows the reminder card for e, beeping every beepInterval until the
// user acts on it. Snoozing shows it again later, until the meeting ends.
func alert(e Event) {
	for {
		switch remind(e) {
		case actionJoin:
			if link := joinURL(e.Link); link != "" {
				if err := openURL(link); err != nil {
					log.Printf("open meeting link: %v", err)
				}
			}
			return
		case actionSnooze:
			time.Sleep(snoozeDuration)
			if time.Now().After(e.End) {
				return
			}
		default:
			return
		}
	}
}

func remind(e Event) string {
	done := make(chan struct{})
	defer close(done)
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

	action, err := showPopup(e)
	if err != nil {
		log.Printf("popup unavailable, using a message box: %v", err)
		messageBox(formatAlert(e, time.Now()), mbOK|mbIconWarning|mbSystemModal|mbSetForeground|mbTopmost)
		return actionDismiss
	}
	return action
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
