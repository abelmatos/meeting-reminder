//go:build !windows

package main

import (
	"context"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// tray has no icon outside Windows; the app runs until interrupted.
type tray struct{}

func newTray(*calendar.ICalSource, string, *reminders) *tray { return &tray{} }

func (*tray) setStatus(Status) {}

func (*tray) run(ctx context.Context, _ *Scheduler) { <-ctx.Done() }
