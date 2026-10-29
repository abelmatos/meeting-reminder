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
