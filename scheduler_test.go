package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu     sync.Mutex
	events []Event
	err    error
	from   time.Time
}

func (f *fakeSource) Events(_ context.Context, from, _ time.Time) ([]Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.from = from
	return f.events, f.err
}

func TestSchedulerNotifiesDueEventsAndTracksSchedule(t *testing.T) {
	now := time.Now()
	soon := Event{ID: "soon", Summary: "Soon", Start: now.Add(5 * time.Minute), End: now.Add(35 * time.Minute)}
	later := Event{ID: "later", Summary: "Later", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)}
	src := &fakeSource{events: []Event{soon, later}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notified := make(chan Event, 2)
	statuses := make(chan Status, 2)
	s := NewScheduler(src, func(e Event) { notified <- e }, func(st Status) { statuses <- st })
	go s.Run(ctx)

	select {
	case e := <-notified:
		if e.ID != "soon" {
			t.Errorf("notified %q, want soon", e.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reminder")
	}
	st := <-statuses
	if st.Next == nil || st.Next.ID != "soon" || st.Err != nil {
		t.Errorf("status = %+v", st)
	}
	if !s.Scheduled("soon") || s.Scheduled("cancelled") {
		t.Error("Scheduled does not reflect the latest fetch")
	}
	src.mu.Lock()
	from := src.from
	src.mu.Unlock()
	if !from.Before(now.Add(-time.Hour)) {
		t.Errorf("fetch window starts at %v; started meetings must stay visible for snooze", from)
	}
}

func TestSchedulerKeepsEventsWhenFetchFails(t *testing.T) {
	now := time.Now()
	src := &fakeSource{err: errors.New("offline")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	statuses := make(chan Status, 1)
	s := NewScheduler(src, func(Event) {}, func(st Status) { statuses <- st })
	s.events = []Event{{ID: "kept", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}}
	go s.Run(ctx)

	st := <-statuses
	if st.Err == nil {
		t.Error("status should carry the fetch error")
	}
	if !s.Scheduled("kept") {
		t.Error("a failed fetch must not drop the previous events")
	}
}
