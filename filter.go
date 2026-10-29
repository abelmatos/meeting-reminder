package main

import (
	"sort"
	"time"
)

// startGrace lets an event that started moments ago still alert, e.g. when the
// app was launched or the PC woke up right after the meeting began.
const startGrace = time.Minute

// maxMeetingLength excludes long timed blocks, such as Google's "Out of office"
// events, which the feed doesn't mark in any other way.
const maxMeetingLength = 12 * time.Hour

// Due returns the events that start within lead of now and have not been
// alerted yet, earliest first. All-day events and blocks of maxMeetingLength or
// more are never due.
func Due(events []Event, now time.Time, lead time.Duration, seen map[string]time.Time) []Event {
	var due []Event
	for _, e := range events {
		if e.AllDay || e.End.Sub(e.Start) >= maxMeetingLength {
			continue
		}
		if _, ok := seen[e.ID]; ok {
			continue
		}
		if e.Start.Before(now.Add(-startGrace)) || e.Start.After(now.Add(lead)) {
			continue
		}
		due = append(due, e)
	}
	sort.Slice(due, func(i, j int) bool { return due[i].Start.Before(due[j].Start) })
	return due
}

// PruneSeen forgets alerted events that started before cutoff, so the map
// doesn't grow forever.
func PruneSeen(seen map[string]time.Time, cutoff time.Time) {
	for id, start := range seen {
		if start.Before(cutoff) {
			delete(seen, id)
		}
	}
}
