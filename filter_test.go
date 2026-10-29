package main

import (
	"testing"
	"time"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	lead := 10 * time.Minute
	ev := func(id string, startOffset time.Duration) Event {
		return Event{ID: id, Summary: id, Start: now.Add(startOffset), End: now.Add(startOffset + 30*time.Minute)}
	}

	tests := []struct {
		name   string
		events []Event
		seen   map[string]time.Time
		want   []string
	}{
		{"inside lead window", []Event{ev("a", 5*time.Minute)}, nil, []string{"a"}},
		{"exactly at lead edge", []Event{ev("a", lead)}, nil, []string{"a"}},
		{"beyond lead window", []Event{ev("a", lead+time.Second)}, nil, nil},
		{"just started, within grace", []Event{ev("a", -30*time.Second)}, nil, []string{"a"}},
		{"started too long ago", []Event{ev("a", -2*time.Minute)}, nil, nil},
		{"already alerted", []Event{ev("a", 5*time.Minute)}, map[string]time.Time{"a": now}, nil},
		{"all-day skipped", []Event{{ID: "a", Start: now.Add(time.Minute), AllDay: true}}, nil, nil},
		{"out-of-office block skipped", []Event{{ID: "a", Start: now.Add(time.Minute), End: now.Add(24 * time.Hour)}}, nil, nil},
		{"sorted by start", []Event{ev("late", 9*time.Minute), ev("early", 2*time.Minute)}, nil, []string{"early", "late"}},
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

func ids(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}
