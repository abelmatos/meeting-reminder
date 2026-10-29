package main

import (
	"bufio"
	"io"
	"log"
	"time"
)

// The card process protocol (see cardproc_windows.go): the child prints
// cardShown once the card is visible, then the user's action, one per line.

const cardShown = "shown"

// readCardResult reads the card process's output until it ends. If the card
// doesn't appear within startTimeout, it calls stop and keeps reading until
// the output closes. It returns the first action, or "" if there was none.
func readCardResult(r io.Reader, startTimeout time.Duration, stop func()) string {
	lines := readLines(r)
	timer := time.NewTimer(startTimeout)
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
				log.Printf("card did not appear within %s; stopping it", startTimeout)
				stop()
			}
		}
	}
	return action
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
