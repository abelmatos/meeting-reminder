//go:build !windows

package main

import "time"

// currentLocal returns the process's local zone. Only Windows caches
// time.Local in a way a clock change does not refresh; see zone_windows.go.
func currentLocal() *time.Location { return time.Local }
