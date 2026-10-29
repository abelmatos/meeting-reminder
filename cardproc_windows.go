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

const (
	cardShown = "shown"
	// cardStartTimeout bounds how long WebView2 may take to show the card
	// before the app gives up and uses a message box instead.
	cardStartTimeout = 20 * time.Second
)

// showCard shows the reminder card for e in a child process and returns the
// action the user chose.
func showCard(e Event) (string, error) {
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
