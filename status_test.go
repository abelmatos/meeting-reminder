package main

import (
	"strings"
	"testing"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

func TestStartStatus(t *testing.T) {
	tests := map[time.Duration]string{
		8 * time.Minute:                 "In 8 min",
		90 * time.Second:                "In 2 min",
		30 * time.Second:                "Starting now",
		-30 * time.Second:               "Started just now",
		-3*time.Minute - 10*time.Second: "Started 3 min ago",
	}
	for d, want := range tests {
		if got := startStatus(d); got != want {
			t.Errorf("startStatus(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestDescribeStatus(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local)
	today := &calendar.Event{Summary: "SRE sync", Start: time.Date(2026, 10, 29, 15, 30, 0, 0, time.Local)}
	tomorrow := &calendar.Event{Summary: "Standup", Start: time.Date(2026, 10, 30, 9, 0, 0, 0, time.Local)}
	long := &calendar.Event{Summary: strings.Repeat("x", 60), Start: today.Start}
	tests := []struct {
		s    Status
		want string
	}{
		{Status{Next: today}, "Next: SRE sync at 15:30"},
		{Status{Next: tomorrow}, "Next: Standup at 09:00 tomorrow"},
		{Status{Next: long}, "Next: " + strings.Repeat("x", 39) + "… at 15:30"},
		{Status{}, "No meetings in the next 24 hours"},
		{Status{Err: errCancelled}, "Can't reach the calendar, retrying"},
		{Status{Next: today, Err: errCancelled}, "Next: SRE sync at 15:30"},
	}
	for _, tt := range tests {
		if got := describeStatus(tt.s, now); got != tt.want {
			t.Errorf("describeStatus = %q, want %q", got, tt.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "event"); got != "1 event" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(4, "event"); got != "4 events" {
		t.Errorf("plural(4) = %q", got)
	}
}

func TestDescribeCalendar(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local)
	events := []calendar.Event{
		{Summary: "SRE sync", Start: now.Add(90 * time.Minute), End: now.Add(2 * time.Hour)},
		{Summary: "Holiday", Start: now, End: now.Add(24 * time.Hour), AllDay: true},
	}
	if got, want := describeCalendar(events, now), `calendar: 2 events in the next 24 hours, next "SRE sync" at Thu 15:30`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := describeCalendar(nil, now), "calendar: 0 events in the next 24 hours"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
