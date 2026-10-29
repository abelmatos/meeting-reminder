//go:build !windows

package main

import (
	"fmt"
	"time"
)

// alert prints the reminder: outside Windows the app can be tried out in WSL
// or Linux during development.
func alert(e Event, _ func(id string) bool) {
	fmt.Printf("\a[ALERT] %s\n", formatAlert(e, time.Now()))
}
