package main

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

func TestCardHTML(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	e := calendar.Event{
		Summary: `<script>alert("x")</script> & sync`,
		Start:   now.Add(8 * time.Minute),
		End:     now.Add(38 * time.Minute),
		Link:    "https://meet.google.com/abc-defg-hij",
	}
	page, err := cardHTML(e)
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
	page, err = cardHTML(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, `data-action="join"`) {
		t.Error("join button shown for a non-https link")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

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
