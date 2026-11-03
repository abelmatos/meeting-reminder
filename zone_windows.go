//go:build windows

package main

import (
	"time"

	"golang.org/x/sys/windows/registry"
)

// currentLocal returns the zone the machine's clock is in right now, re-reading
// the Windows registry on every call. Go loads time.Local once at startup, so
// after a time-zone change (travelling, or the user editing the clock) the card
// and the tray kept showing the old zone's wall times until the app restarted.
//
// The active bias carries the daylight-saving shift, and Windows updates it at
// a transition, so the same re-read follows those too.
func currentLocal() *time.Location {
	const path = `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return time.Local
	}
	defer key.Close()
	name, _, err := key.GetStringValue("TimeZoneKeyName")
	if err != nil || name == "" {
		return time.Local
	}
	// Windows reports the bias as UTC minus local time, in minutes.
	bias, _, err := key.GetIntegerValue("ActiveTimeBias")
	if err != nil {
		return time.Local
	}
	return time.FixedZone(name, -int(int32(bias))*60)
}
