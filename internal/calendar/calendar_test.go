package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testFeed mimics the shape of a Google Calendar export. 2026-10-25 is the
// end of daylight saving time in Europe/Zurich.
const testFeed = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Google Inc//Google Calendar 70.9054//EN
X-WR-CALNAME:me@example.com
BEGIN:VEVENT
UID:single
DTSTART;TZID=Europe/Zurich:20261029T150000
DTEND;TZID=Europe/Zurich:20261029T153000
SUMMARY:  Single 
X-GOOGLE-CONFERENCE:https://meet.google.com/abc-defg-hij
END:VEVENT
BEGIN:VEVENT
UID:allday
DTSTART;VALUE=DATE:20261029
DTEND;VALUE=DATE:20261030
SUMMARY:Holiday
END:VEVENT
BEGIN:VEVENT
UID:weekly
DTSTART;TZID=Europe/Zurich:20261001T100000
DTEND;TZID=Europe/Zurich:20261001T103000
RRULE:FREQ=WEEKLY;WKST=SU;BYDAY=TH
EXDATE;TZID=Europe/Zurich:20261008T100000,20261015T100000
SUMMARY:Weekly
END:VEVENT
BEGIN:VEVENT
UID:weekly
RECURRENCE-ID;TZID=Europe/Zurich:20261022T100000
DTSTART;TZID=Europe/Zurich:20261022T113000
DTEND;TZID=Europe/Zurich:20261022T120000
SUMMARY:Weekly (moved)
END:VEVENT
BEGIN:VEVENT
UID:weekly
RECURRENCE-ID;TZID=Europe/Zurich:20261029T100000
DTSTART;TZID=Europe/Zurich:20261029T100000
DTEND;TZID=Europe/Zurich:20261029T103000
STATUS:CANCELLED
SUMMARY:Weekly
END:VEVENT
BEGIN:VEVENT
UID:declined
DTSTART:20261029T130000Z
DTEND:20261029T133000Z
ATTENDEE;CN=Me;PARTSTAT=DECLINED:mailto:Me@Example.com
ATTENDEE;PARTSTAT=ACCEPTED:mailto:other@example.com
SUMMARY:Declined
END:VEVENT
BEGIN:VEVENT
UID:declined-by-other
DTSTART:20261029T140000Z
DTEND:20261029T143000Z
ATTENDEE;PARTSTAT=ACCEPTED:mailto:me@example.com
ATTENDEE;PARTSTAT=DECLINED:mailto:other@example.com
SUMMARY:Others declined
END:VEVENT
BEGIN:VEVENT
UID:cancelled
DTSTART:20261029T160000Z
DTEND:20261029T163000Z
STATUS:CANCELLED
SUMMARY:Cancelled
END:VEVENT
BEGIN:VEVENT
UID:located
DTSTART:20261029T170000Z
DTEND:20261029T173000Z
LOCATION:https://zoom.us/j/123?pwd=abc\, Room A
END:VEVENT
BEGIN:VEVENT
UID:outside
DTSTART:20261201T090000Z
DTEND:20261201T093000Z
SUMMARY:Outside window
END:VEVENT
END:VCALENDAR
`

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func TestParseEvents(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	events, warnings, err := ParseEvents(strings.NewReader(crlf(testFeed)), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	allDayStart := time.Date(2026, 10, 29, 0, 0, 0, 0, time.Local)
	want := map[string]string{
		"single/2026-10-29T14:00:00Z":                      "Single",
		"allday/" + allDayStart.UTC().Format(time.RFC3339): "Holiday",
		"weekly/2026-10-01T08:00:00Z":                      "Weekly",         // CEST
		"weekly/2026-10-22T09:30:00Z":                      "Weekly (moved)", // override
		"weekly/2026-11-05T09:00:00Z":                      "Weekly",         // CET, after DST ends
		"declined-by-other/2026-10-29T14:00:00Z":           "Others declined",
		"located/2026-10-29T17:00:00Z":                     "(No title)",
	}
	got := map[string]string{}
	for _, e := range events {
		got[e.ID] = e.Summary
	}
	for id, summary := range want {
		if got[id] != summary {
			t.Errorf("missing %s (%q)", id, summary)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected event %s (%q)", id, got[id])
		}
	}

	for _, e := range events {
		switch {
		case e.ID == "single/2026-10-29T14:00:00Z":
			if e.Link != "https://meet.google.com/abc-defg-hij" {
				t.Errorf("link = %q", e.Link)
			}
			if e.End.Sub(e.Start) != 30*time.Minute {
				t.Errorf("duration = %v", e.End.Sub(e.Start))
			}
		case strings.HasPrefix(e.ID, "located/"):
			if e.Link != "https://zoom.us/j/123?pwd=abc" {
				t.Errorf("location link = %q", e.Link)
			}
		case strings.HasPrefix(e.ID, "allday/"):
			if !e.AllDay {
				t.Error("all-day event not marked AllDay")
			}
		case e.AllDay:
			t.Errorf("%s wrongly marked AllDay", e.ID)
		}
	}
}

func TestParseEventsSkipsBrokenEvent(t *testing.T) {
	feed := crlf(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:broken
DTSTART;TZID=Not/AZone:20261029T150000
SUMMARY:Broken
END:VEVENT
BEGIN:VEVENT
UID:ok
DTSTART:20261029T150000Z
SUMMARY:OK
END:VEVENT
END:VCALENDAR
`)
	from := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	events, warnings, err := ParseEvents(strings.NewReader(feed), from, from.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want 1", warnings)
	}
	if len(events) != 1 || events[0].Summary != "OK" {
		t.Errorf("events = %v", ids(events))
	}
}

func TestICalSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private-secret/basic.ics" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(crlf(testFeed)))
	}))
	defer srv.Close()

	from := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	src := &ICalSource{Client: srv.Client()}
	src.SetURL(srv.URL + "/private-secret/basic.ics")
	events, err := src.Events(context.Background(), from, from.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("no events")
	}

	src.SetURL(srv.URL + "/private-secret/missing.ics")
	if _, err := src.Events(context.Background(), from, from.Add(time.Hour)); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Errorf("HTTP error should be reported without the URL, got %v", err)
	}

	srv.Close()
	src.SetURL(srv.URL + "/private-secret/basic.ics")
	if _, err := src.Events(context.Background(), from, from.Add(time.Hour)); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Errorf("network error should be reported without the URL, got %v", err)
	}
}

func TestValidateFeedURL(t *testing.T) {
	tests := []struct {
		url string
		ok  bool
	}{
		{"https://calendar.google.com/calendar/ical/me%40example.com/private-abc/basic.ics", true},
		{"  https://calendar.google.com/calendar/ical/x/private-abc/basic.ics\n", true},
		{"http://calendar.google.com/calendar/ical/x/private-abc/basic.ics", false},
		{"https://calendar.google.com/calendar/embed?src=x", false},
		{"not a url", false},
		{"", false},
	}
	for _, tt := range tests {
		if err := ValidateFeedURL(tt.url); (err == nil) != tt.ok {
			t.Errorf("ValidateFeedURL(%q) = %v, want ok=%v", tt.url, err, tt.ok)
		}
	}
}

func ids(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}
