//go:build !windows

package main

import "context"

// tray has no icon outside Windows; the app runs until interrupted.
type tray struct{}

func newTray(*ICalSource, string) *tray { return &tray{} }

func (*tray) setStatus(Status) {}

func (*tray) run(ctx context.Context, _ *Scheduler) { <-ctx.Done() }
