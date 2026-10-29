//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/danieljoos/wincred"
	"golang.org/x/sys/windows"
)

const credentialTarget = "meeting-reminder/ical-url"

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

// acquireSingleInstance reports whether this is the only running instance. The
// mutex handle is left open on purpose: Windows releases it when the process exits.
func acquireSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\meeting-reminder`)
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return false
	}
	return true
}

// loadFeedURL reads the calendar URL from Windows Credential Manager.
func loadFeedURL() (string, error) {
	cred, err := wincred.GetGenericCredential(credentialTarget)
	if err != nil {
		return "", err
	}
	return string(cred.CredentialBlob), nil
}

// saveFeedURL stores the calendar URL in Windows Credential Manager, which
// encrypts it for the current Windows user.
func saveFeedURL(u string) error {
	cred := wincred.NewGenericCredential(credentialTarget)
	cred.UserName = "ical-url"
	cred.CredentialBlob = []byte(u)
	cred.Persist = wincred.PersistLocalMachine
	return cred.Write()
}

// promptFeedURL asks the user to copy the secret iCal address, then takes it
// from the clipboard and clears the clipboard.
func promptFeedURL() (string, error) {
	ok := confirm("Setup: in Google Calendar open Settings → your calendar → Integrate calendar, " +
		"copy the \"Secret address in iCal format\", then click OK.\n\n" +
		"The address is read from the clipboard, stored in Windows Credential Manager, " +
		"and the clipboard is cleared.")
	if !ok {
		return "", errCancelled
	}
	text, err := takeClipboardText()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func takeClipboardText() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, err := procOpenClipboard.Call(0); r == 0 {
		return "", fmt.Errorf("open clipboard: %w", err)
	}
	defer func() { _, _, _ = procCloseClipboard.Call() }()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", errors.New("the clipboard has no text")
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return "", fmt.Errorf("lock clipboard: %w", err)
	}
	// p is a pointer returned by GlobalLock; reinterpret it without a
	// uintptr-to-pointer conversion, which go vet rejects.
	text := windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&p)))
	_, _, _ = procGlobalUnlock.Call(h)
	_, _, _ = procEmptyClipboard.Call()
	return text, nil
}
