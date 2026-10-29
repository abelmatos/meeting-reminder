//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

// alert prints the reminder: outside Windows the app can be tried out in WSL
// or Linux during development.
func alert(_ context.Context, e calendar.Event, _ func(id string) bool) {
	fmt.Printf("\a[ALERT] %s\n", formatAlert(e, time.Now()))
}
