package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// Status is what the app knows about the calendar, shown in the tray.
type Status struct {
	Next *Event // next meeting that will alert, if any
	Err  error  // last fetch error; nil once a fetch succeeds
}

// Scheduler keeps the calendar fresh and starts a reminder for each meeting
// shortly before it begins.
type Scheduler struct {
	src EventSource
	// notify shows a reminder; each call runs in its own goroutine.
	notify func(Event)
	// onStatus, if set, receives the state after every check.
	onStatus func(Status)
	refresh  chan struct{}

	mu     sync.Mutex
	events []Event // latest successful fetch
}

func NewScheduler(src EventSource, notify func(Event), onStatus func(Status)) *Scheduler {
	return &Scheduler{src: src, notify: notify, onStatus: onStatus, refresh: make(chan struct{}, 1)}
}

// Refresh makes Run fetch the calendar at its next opportunity.
func (s *Scheduler) Refresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

// Scheduled reports whether the latest fetch still contains the event with
// this ID. Moving a meeting changes its ID; cancelling or declining it
// removes it.
func (s *Scheduler) Scheduled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.events {
		if e.ID == id {
			return true
		}
	}
	return false
}

// Run fetches events every fetchInterval (or after Refresh) and notifies about
// due events every checkInterval until ctx is cancelled. A failed fetch keeps
// the last events.
func (s *Scheduler) Run(ctx context.Context) {
	var events []Event
	var lastFetch time.Time
	var fetchErr error
	var failures int
	var logged string // last calendar summary written to the log
	seen := map[string]time.Time{}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		now := time.Now()
		if now.Sub(lastFetch) >= fetchInterval {
			// Include meetings that already started, so a reminder snoozed
			// past the start can still tell that its meeting is on.
			evs, err := s.src.Events(ctx, now.Add(-maxMeetingLength), now.Add(lookahead))
			if fetchErr = err; err != nil {
				// Failed fetches retry every check; log the first and then
				// every 20th (~10 minutes) so an outage doesn't flood the log.
				if failures++; failures == 1 || failures%20 == 0 {
					log.Printf("fetch failed (%s in a row): %v", plural(failures, "attempt"), err)
				}
			} else {
				if failures > 0 {
					log.Printf("fetch recovered after %s", plural(failures, "failed attempt"))
				}
				failures = 0
				events, lastFetch = evs, now
				s.mu.Lock()
				s.events = evs
				s.mu.Unlock()
				if d := describeCalendar(events, now); d != logged {
					log.Print(d)
					logged = d
				}
			}
		}
		for _, e := range Due(events, now, leadTime, seen) {
			seen[e.ID] = e.Start
			log.Printf("alerting %q at %s", e.Summary, e.Start.Format(time.RFC3339))
			go s.notify(e)
		}
		PruneSeen(seen, now.Add(-time.Hour))
		if s.onStatus != nil {
			s.onStatus(Status{Next: NextEvent(events, now), Err: fetchErr})
		}

		select {
		case <-ctx.Done():
			return
		case <-s.refresh:
			lastFetch = time.Time{}
		case <-ticker.C:
		}
	}
}
