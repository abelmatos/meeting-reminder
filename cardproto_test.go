package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadCardResult(t *testing.T) {
	tests := []struct {
		name, output, want string
	}{
		{"shown then action", "shown\nsnooze\n", actionSnooze},
		{"first action wins", "shown\njoin\ndismiss\n", actionJoin},
		{"unknown lines are ignored", "shown\nhello\ndismiss\n", actionDismiss},
		{"no answer", "shown\n", ""},
		{"no output", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopped := false
			got := readCardResult(strings.NewReader(tt.output), time.Second, func() { stopped = true })
			if got != tt.want {
				t.Errorf("action = %q, want %q", got, tt.want)
			}
			if stopped {
				t.Error("stop called although the output ended in time")
			}
		})
	}
}

func TestReadCardResultStopsCardThatNeverAppears(t *testing.T) {
	r, w := io.Pipe()
	stopped := make(chan struct{})
	result := make(chan string, 1)
	go func() {
		result <- readCardResult(r, 10*time.Millisecond, func() {
			close(stopped)
			_ = w.Close() // like killing the process: its output ends
		})
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop was not called for a card that never appeared")
	}
	if got := <-result; got != "" {
		t.Errorf("action = %q, want none", got)
	}
}
