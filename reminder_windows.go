//go:build windows

package main

import (
	"context"
	"log"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// beepInterval is how often a reminder beeps while it is open.
const beepInterval = 30 * time.Second

func platformReminderUI() reminderUI {
	return reminderUI{show: remind, open: openURL, snooze: snoozeDuration, now: time.Now}
}

// remind shows the card for e once, beeping every beepInterval while it is
// open, and falls back to a message box if the card fails.
func remind(ctx context.Context, e calendar.Event) string {
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(beepInterval)
		defer t.Stop()
		for {
			_, _, _ = procMessageBeep.Call(mbIconWarning)
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()

	action, err := showCard(ctx, e)
	if ctx.Err() != nil {
		return actionDismiss // the card was closed because the app is exiting
	}
	if err != nil {
		log.Printf("reminder card failed, using a message box: %v", err)
		messageBox(formatAlert(e, time.Now()), mbOK|mbIconWarning|mbSystemModal|mbSetForeground|mbTopmost)
		return actionDismiss
	}
	return action
}
