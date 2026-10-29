package main

import (
	"testing"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	lead := 10 * time.Minute
	ev := func(id string, startOffset time.Duration) calendar.Event {
		return calendar.Event{ID: id, Summary: id, Start: now.Add(startOffset), End: now.Add(startOffset + 30*time.Minute)}
	}

	tests := []struct {
		name   string
		events []calendar.Event
		seen   map[string]time.Time
		want   []string
	}{
		{"inside lead window", []calendar.Event{ev("a", 5*time.Minute)}, nil, []string{"a"}},
		{"exactly at lead edge", []calendar.Event{ev("a", lead)}, nil, []string{"a"}},
		{"beyond lead window", []calendar.Event{ev("a", lead+time.Second)}, nil, nil},
		{"just started, within grace", []calendar.Event{ev("a", -30*time.Second)}, nil, []string{"a"}},
		{"started too long ago", []calendar.Event{ev("a", -2*time.Minute)}, nil, nil},
		{"already alerted", []calendar.Event{ev("a", 5*time.Minute)}, map[string]time.Time{"a": now}, nil},
		{"all-day skipped", []calendar.Event{{ID: "a", Start: now.Add(time.Minute), AllDay: true}}, nil, nil},
		{"out-of-office block skipped", []calendar.Event{{ID: "a", Start: now.Add(time.Minute), End: now.Add(24 * time.Hour)}}, nil, nil},
		{"sorted by start", []calendar.Event{ev("late", 9*time.Minute), ev("early", 2*time.Minute)}, nil, []string{"early", "late"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := tt.seen
			if seen == nil {
				seen = map[string]time.Time{}
			}
			got := Due(tt.events, now, lead, seen)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d events %v, want %v", len(got), ids(got), tt.want)
			}
			for i := range got {
				if got[i].ID != tt.want[i] {
					t.Errorf("got %v, want %v", ids(got), tt.want)
				}
			}
		})
	}
}

func TestPruneSeen(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	seen := map[string]time.Time{
		"old": now.Add(-2 * time.Hour),
		"new": now.Add(-time.Minute),
	}
	PruneSeen(seen, now.Add(-time.Hour))
	if _, ok := seen["old"]; ok {
		t.Error("old entry not pruned")
	}
	if _, ok := seen["new"]; !ok {
		t.Error("new entry pruned")
	}
}

func ids(events []calendar.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

func TestNextEvent(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	ev := func(id string, startOffset time.Duration) calendar.Event {
		return calendar.Event{ID: id, Start: now.Add(startOffset), End: now.Add(startOffset + 30*time.Minute)}
	}
	events := []calendar.Event{
		ev("started", -time.Minute),
		ev("later", 3*time.Hour),
		{ID: "allday", Start: now.Add(time.Hour), End: now.Add(25 * time.Hour), AllDay: true},
		{ID: "ooo", Start: now.Add(time.Hour), End: now.Add(13 * time.Hour)},
		ev("soon", 2*time.Hour),
	}
	if got := NextEvent(events, now); got == nil || got.ID != "soon" {
		t.Errorf("NextEvent = %v, want soon", got)
	}
	if got := NextEvent(events[:1], now); got != nil {
		t.Errorf("NextEvent = %v, want nil", got)
	}
}
