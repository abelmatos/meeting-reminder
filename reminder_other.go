//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// Outside Windows reminders are printed, so the app can be tried out in WSL
// or Linux during development.
func platformReminderUI() reminderUI {
	return reminderUI{
		show: func(_ context.Context, e calendar.Event) string {
			fmt.Printf("\a[ALERT] %s\n", formatAlert(e, time.Now()))
			return actionDismiss
		},
		open:   func(string) error { return nil },
		snooze: snoozeDuration,
		now:    time.Now,
	}
}
