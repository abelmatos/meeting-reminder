package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// shutdownTimeout bounds how long Exit waits for open cards to close.
const shutdownTimeout = 5 * time.Second

// Actions a reminder card can return; card.html posts them.
const (
	actionJoin    = "join"
	actionSnooze  = "snooze"
	actionDismiss = "dismiss"
)

// reminderUI holds the platform-specific parts of a reminder.
type reminderUI struct {
	// show presents the reminder once and returns the user's action.
	show func(ctx context.Context, e calendar.Event) string
	// open opens a meeting link.
	open func(link string) error
	// snooze is how long a snoozed reminder waits before showing again.
	snooze time.Duration
	now    func() time.Time
}

// alert shows a reminder for e until the user acts on it. A snoozed reminder
// comes back while stillScheduled reports the meeting is still on and it
// hasn't ended. Cancelling ctx closes the reminder.
func alert(ctx context.Context, e calendar.Event, stillScheduled func(id string) bool) {
	platformReminderUI().run(ctx, e, stillScheduled)
}

func (u reminderUI) run(ctx context.Context, e calendar.Event, stillScheduled func(id string) bool) {
	for {
		action := u.show(ctx, e)
		if ctx.Err() != nil {
			log.Printf("reminder %q: closed, the app is exiting", e.Summary)
			return
		}
		log.Printf("reminder %q: %s", e.Summary, action)
		switch action {
		case actionJoin:
			if link := joinURL(e.Link); link != "" {
				if err := u.open(link); err != nil {
					log.Printf("open meeting link: %v", err)
				}
			}
			return
		case actionSnooze:
			t := time.NewTimer(snoozeWait(u.now(), e.Start, u.snooze))
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			if u.now().After(e.End) || !stillScheduled(e.ID) {
				log.Printf("reminder %q: not shown again, the meeting ended, moved or was cancelled", e.Summary)
				return
			}
		default:
			return
		}
	}
}

// snoozeWait is how long a reminder snoozed at now waits before showing
// again. Before the meeting starts it never waits past the start, so a late
// snooze doesn't make the user join late. Once the meeting has started it
// waits the full snooze.
func snoozeWait(now, start time.Time, snooze time.Duration) time.Duration {
	if untilStart := start.Sub(now); untilStart > 0 && untilStart < snooze {
		return untilStart
	}
	return snooze
}

// reminders runs reminder goroutines tied to the app's context, so Exit
// closes open cards and drops snoozed reminders instead of leaving them
// behind.
type reminders struct {
	ctx context.Context
	wg  sync.WaitGroup
}

func newReminders(ctx context.Context) *reminders {
	return &reminders{ctx: ctx}
}

// start shows a reminder for e in the background.
func (r *reminders) start(e calendar.Event, stillScheduled func(id string) bool) {
	r.wg.Go(func() { alert(r.ctx, e, stillScheduled) })
}

// wait waits, up to timeout, for reminders to finish after the context is
// cancelled, and reports whether they did.
func (r *reminders) wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		log.Printf("reminders still open after %s; exiting anyway", timeout)
		return false
	}
}
