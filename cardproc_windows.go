//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// Parent side of the reminder card. Each card runs in a child process
// (reminder.exe -card, see cardwindow_windows.go). WebView2
// failures inside go-webview2 can exit or panic the process, hang, or leak
// the browser, and a child process contains all three: the app falls back to
// a message box, and exiting the child frees WebView2.
//
// Protocol: the parent writes the Event as JSON to the child's stdin. The
// child prints cardShown once the card is visible, then the chosen action,
// and logs to stderr, which the parent copies into its log.

// cardStartTimeout bounds how long WebView2 may take to show the card before
// the app gives up and uses a message box instead.
const cardStartTimeout = 20 * time.Second

// showCard shows the reminder card for e in a child process and returns the
// action the user chose. Cancelling ctx closes the card.
func showCard(ctx context.Context, e calendar.Event) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	input, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-card") //nolint:gosec // G204: re-runs this executable
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = log.Writer()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	action := readCardResult(stdout, cardStartTimeout, cancel)
	waitErr := cmd.Wait()
	if action == "" {
		if waitErr != nil {
			return "", fmt.Errorf("card exited without an answer: %w", waitErr)
		}
		return "", errors.New("card exited without an answer")
	}
	return action, nil
}
