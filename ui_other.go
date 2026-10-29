//go:build !windows

package main

import "context"

// runUI runs the reminder loop; there is no tray icon outside Windows.
func runUI(ctx context.Context, _ context.CancelFunc, src *ICalSource) {
	run(ctx, src, alert, nil, nil)
}
