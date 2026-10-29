package main

import (
	"sort"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// startGrace lets an event that started moments ago still alert, e.g. when the
// app was launched or the PC woke up right after the meeting began.
const startGrace = time.Minute

// maxMeetingLength excludes long timed blocks, such as Google's "Out of office"
// events, which the feed doesn't mark in any other way.
const maxMeetingLength = 12 * time.Hour

// alertable reports whether e is the kind of event that gets a reminder.
func alertable(e calendar.Event) bool {
	return !e.AllDay && e.End.Sub(e.Start) < maxMeetingLength
}

// Due returns the events that start within lead of now and have not been
// alerted yet, earliest first. All-day events and blocks of maxMeetingLength or
// more are never due.
func Due(events []calendar.Event, now time.Time, lead time.Duration, seen map[string]time.Time) []calendar.Event {
	var due []calendar.Event
	for _, e := range events {
		if !alertable(e) {
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

// NextEvent returns the earliest event that hasn't started yet and would
// alert, or nil if there is none.
func NextEvent(events []calendar.Event, now time.Time) *calendar.Event {
	var next *calendar.Event
	for i, e := range events {
		if !alertable(e) || !e.Start.After(now) {
			continue
		}
		if next == nil || e.Start.Before(next.Start) {
			next = &events[i]
		}
	}
	return next
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
