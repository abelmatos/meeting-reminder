//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

// Each reminder card runs in a child process (reminder.exe -card). WebView2
// failures inside go-webview2 can exit or panic the process, hang, or leak
// the browser, and a child process contains all three: the app falls back to
// a message box, and exiting the child frees WebView2.
//
// Protocol: the parent writes the Event as JSON to the child's stdin. The
// child prints cardShown once the card is visible, then the chosen action,
// and logs to stderr, which the parent copies into its log.

const (
	cardShown = "shown"
	// cardStartTimeout bounds how long WebView2 may take to show the card
	// before the app gives up and uses a message box instead.
	cardStartTimeout = 20 * time.Second
)

// cardOptions changes how the card is rendered.
type cardOptions struct {
	// variant selects a troubleshooting variant (TEMPORARY, see cardVariants).
	variant string
}

// cardVariants are TEMPORARY troubleshooting variants for cards that show up
// empty, offered in the tray's Troubleshooting menu.
var cardVariants = []struct{ id, title string }{
	{"refresh", "Re-show the page after the window appears"},
	{"show-first", "Show the window before loading the page"},
	{"no-occlusion", "Turn off occlusion detection"},
	{"no-gpu", "Render without the GPU"},
}

// showCard shows the reminder card for e in a child process and returns the
// action the user chose.
func showCard(e Event, opts cardOptions) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	input, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-card", "-card-variant", opts.variant) //nolint:gosec // G204: re-runs this executable
	switch opts.variant {
	case "no-gpu":
		cmd.Env = append(os.Environ(), "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--disable-gpu")
	case "no-occlusion":
		cmd.Env = append(os.Environ(), "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--disable-features=CalculateNativeWinOcclusion")
	}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = log.Writer()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	lines := readLines(stdout)
	timer := time.NewTimer(cardStartTimeout)
	defer timer.Stop()
	shown, action := false, ""
	for lines != nil {
		select {
		case line, ok := <-lines:
			switch {
			case !ok:
				lines = nil
			case line == cardShown:
				shown = true
			case action == "" && (line == actionJoin || line == actionSnooze || line == actionDismiss):
				action = line
			}
		case <-timer.C:
			if !shown {
				log.Printf("card did not appear within %s; stopping it", cardStartTimeout)
				cancel()
			}
		}
	}
	waitErr := cmd.Wait()
	if action == "" {
		if waitErr != nil {
			return "", fmt.Errorf("card exited without an answer: %w", waitErr)
		}
		return "", errors.New("card exited without an answer")
	}
	return action, nil
}

func readLines(r io.Reader) <-chan string {
	lines := make(chan string, 4)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines
}

// runCard is the child side: it shows one card and prints the result.
func runCard(variant string) int {
	log.SetOutput(os.Stderr)
	log.SetPrefix("card: ")

	// Resolve implicitly loaded DLLs from System32 only, so a planted
	// WebView2Loader.dll next to the exe (e.g. in Downloads) is never loaded;
	// go-webview2 then uses its embedded loader.
	if err := windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32); err != nil {
		log.Printf("restrict DLL search: %v", err)
		return 1
	}

	var e Event
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&e); err != nil {
		log.Printf("read event: %v", err)
		return 1
	}
	page, err := popupHTML(e)
	if err != nil {
		log.Printf("render card: %v", err)
		return 1
	}
	// go-webview2's edge package locked the main goroutine to the main
	// thread and initialised COM on it, which is where the card must run.
	if variant != "" {
		log.Printf("variant %q", variant)
	}
	action, err := runPopup(page, variant, func() { fmt.Println(cardShown) })
	if err != nil {
		log.Print(err)
		return 1
	}
	fmt.Println(action)
	return 0
}
