//go:build !windows

package main

import "context"

// runUI runs the reminder loop; there is no tray icon outside Windows.
func runUI(ctx context.Context, _ context.CancelFunc, src *ICalSource) {
	var s *Scheduler
	s = NewScheduler(src, func(e Event) { alert(e, s.Scheduled) }, nil)
	s.Run(ctx)
}
