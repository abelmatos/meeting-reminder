//go:build !windows

package main

import "time"

// localTime converts an instant to the process's local wall time. Only Windows
// needs to re-read the zone per call; see zone_windows.go.
func localTime(t time.Time) time.Time { return t.In(time.Local) }
