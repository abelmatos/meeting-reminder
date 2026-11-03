//go:build windows

package main

import (
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// currentLocal re-reads the registry instead of trusting time.Local (#7): the
// zone name must be the one Windows reports now, and the offset must match the
// clock right now, daylight saving included.
func TestCurrentLocalMatchesTheRegistryZone(t *testing.T) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\TimeZoneInformation`, registry.QUERY_VALUE)
	if err != nil {
		t.Skipf("cannot open the time zone key: %v", err)
	}
	defer key.Close()
	want, _, err := key.GetStringValue("TimeZoneKeyName")
	if err != nil || want == "" {
		t.Skipf("no TimeZoneKeyName: %v", err)
	}

	got := currentLocal()
	if got.String() != want {
		t.Fatalf("currentLocal() = %q, want %q", got.String(), want)
	}

	now := time.Now()
	_, wantOffset := now.Zone()
	_, gotOffset := now.In(got).Zone()
	if gotOffset != wantOffset {
		t.Fatalf("currentLocal offset = %d, want %d", gotOffset, wantOffset)
	}
}
