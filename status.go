package main

import (
	"fmt"
	"time"
)

// User-facing wording: the tray's status line, the calendar summary in the
// log, and the plain-text reminder used when the card can't be shown.

// describeCalendar summarises a fetch for the log; it is logged only when it
// changes.
func describeCalendar(events []Event, now time.Time) string {
	upcoming := 0
	for _, e := range events {
		if !e.Start.Before(now) {
			upcoming++
		}
	}
	s := fmt.Sprintf("calendar: %s in the next %s", plural(upcoming, "event"), lookaheadText())
	if next := NextEvent(events, now); next != nil {
		s += fmt.Sprintf(", next %q at %s", next.Summary, next.Start.Local().Format("Mon 15:04"))
	}
	return s
}

// describeStatus is the one-line summary shown in the tray.
func describeStatus(s Status, now time.Time) string {
	if s.Next == nil {
		if s.Err != nil {
			return "Can't reach the calendar, retrying"
		}
		return "No meetings in the next " + lookaheadText()
	}
	start := s.Next.Start.Local()
	day := ""
	if sameDay(start, now.AddDate(0, 0, 1)) {
		day = " tomorrow"
	}
	summary := s.Next.Summary
	if r := []rune(summary); len(r) > 40 {
		summary = string(r[:39]) + "…"
	}
	return fmt.Sprintf("Next: %s at %s%s", summary, start.Format("15:04"), day)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func lookaheadText() string {
	return plural(int(lookahead.Hours()), "hour")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// formatAlert is the plain-text reminder, used where the card isn't.
func formatAlert(e Event, now time.Time) string {
	msg := fmt.Sprintf("%s\n\n%s · %s", e.Summary, startStatus(e.Start.Sub(now)), e.Start.Local().Format("15:04"))
	if e.Link != "" {
		msg += "\n\n" + e.Link
	}
	return msg
}

// startStatus is the countdown text for the message-box fallback; card.html
// computes the same text in JavaScript.
func startStatus(untilStart time.Duration) string {
	switch {
	case untilStart >= time.Minute:
		return fmt.Sprintf("In %d min", int(untilStart.Round(time.Minute).Minutes()))
	case untilStart > 0:
		return "Starting now"
	case untilStart > -time.Minute:
		return "Started just now"
	default:
		return fmt.Sprintf("Started %d min ago", int(-untilStart.Minutes()))
	}
}
