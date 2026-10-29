package main

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPopupHTML(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	e := Event{
		Summary: `<script>alert("x")</script> & sync`,
		Start:   now.Add(8 * time.Minute),
		End:     now.Add(38 * time.Minute),
		Link:    "https://meet.google.com/abc-defg-hij",
	}
	page, err := popupHTML(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; sync",
		"Google Meet",
		`data-action="join"`,
		"Snooze 5 min",
		"const start =  " + itoa(e.Start.UnixMilli()) + " ;",
		"const hasLink =  true ;",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	if strings.Contains(page, `<script>alert("x")`) {
		t.Error("summary is not escaped")
	}

	e.Link = "javascript:alert(1)"
	page, err = popupHTML(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, `data-action="join"`) {
		t.Error("join button shown for a non-https link")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

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

func TestJoinURLAndProvider(t *testing.T) {
	tests := []struct{ link, join, provider string }{
		{"https://meet.google.com/abc-defg-hij", "https://meet.google.com/abc-defg-hij", "Google Meet"},
		{"https://acme.zoom.us/j/123", "https://acme.zoom.us/j/123", "Zoom"},
		{"https://teams.microsoft.com/l/meetup-join/x", "https://teams.microsoft.com/l/meetup-join/x", "Microsoft Teams"},
		{"https://example.com/call", "https://example.com/call", "Video call"},
		{"http://meet.google.com/abc", "", "Google Meet"},
		{"file:///C:/Windows/System32/calc.exe", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := joinURL(tt.link); got != tt.join {
			t.Errorf("joinURL(%q) = %q, want %q", tt.link, got, tt.join)
		}
		if got := meetingProvider(tt.link); got != tt.provider {
			t.Errorf("meetingProvider(%q) = %q, want %q", tt.link, got, tt.provider)
		}
	}
}

func TestDescribeStatus(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local)
	today := &Event{Summary: "SRE sync", Start: time.Date(2026, 10, 29, 15, 30, 0, 0, time.Local)}
	tomorrow := &Event{Summary: "Standup", Start: time.Date(2026, 10, 30, 9, 0, 0, 0, time.Local)}
	long := &Event{Summary: strings.Repeat("x", 60), Start: today.Start}
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
	events := []Event{
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
