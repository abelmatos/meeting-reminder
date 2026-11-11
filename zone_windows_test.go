//go:build windows

package main

import (
	"testing"
	"time"
	_ "time/tzdata" // reference zones for the comparison below
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procEnumDynamicTimeZoneInformation = windows.NewLazySystemDLL("advapi32.dll").NewProc("EnumDynamicTimeZoneInformation")
	procGetLocalTime                   = kernel32.NewProc("GetLocalTime")
)

// The size alone can't catch swapped fields, so pin every offset to
// timezoneapi.h.
func TestDynamicTimeZoneInformationLayout(t *testing.T) {
	var tz dynamicTimeZoneInformation
	for _, f := range []struct {
		name      string
		got, want uintptr
	}{
		{"StandardName", unsafe.Offsetof(tz.StandardName), 4},
		{"StandardDate", unsafe.Offsetof(tz.StandardDate), 68},
		{"StandardBias", unsafe.Offsetof(tz.StandardBias), 84},
		{"DaylightName", unsafe.Offsetof(tz.DaylightName), 88},
		{"DaylightDate", unsafe.Offsetof(tz.DaylightDate), 152},
		{"DaylightBias", unsafe.Offsetof(tz.DaylightBias), 168},
		{"TimeZoneKeyName", unsafe.Offsetof(tz.TimeZoneKeyName), 172},
		{"DynamicDaylightTimeDisabled", unsafe.Offsetof(tz.DynamicDaylightTimeDisabled), 428},
		{"sizeof", unsafe.Sizeof(tz), 432},
	} {
		if f.got != f.want {
			t.Errorf("%s at %d, want %d", f.name, f.got, f.want)
		}
	}
}

// localTime must agree with the wall clock Windows keeps right now.
func TestLocalTimeMatchesTheClock(t *testing.T) {
	var st windows.Systemtime
	procGetLocalTime.Call(uintptr(unsafe.Pointer(&st))) //nolint:errcheck,gosec // GetLocalTime has no failure value
	got := localTime(time.Now())
	want := time.Date(int(st.Year), time.Month(st.Month), int(st.Day),
		int(st.Hour), int(st.Minute), int(st.Second), 0, got.Location())
	if d := got.Sub(want); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("localTime(now) = %s, Windows clock says %s", got.Format(time.DateTime), want.Format(time.DateTime))
	}

	var tz dynamicTimeZoneInformation
	if err := getDynamicTimeZoneInformation(&tz); err != nil {
		t.Fatal(err)
	}
	if windows.UTF16ToString(tz.TimeZoneKeyName[:]) == "" {
		t.Error("current zone has no key name")
	}
}

// zoneAt must give every instant the offset its own date has, on both sides
// of each daylight-saving switch, in both hemispheres and without DST.
func TestZoneAtMatchesTzdata(t *testing.T) {
	for key, iana := range map[string]string{
		"GMT Standard Time":         "Europe/London",
		"W. Europe Standard Time":   "Europe/Berlin",
		"Pacific Standard Time":     "America/Los_Angeles",
		"AUS Eastern Standard Time": "Australia/Sydney",
		"Tokyo Standard Time":       "Asia/Tokyo",
	} {
		tz, ok := findZone(key)
		if !ok {
			t.Errorf("%s not in the Windows zone list", key)
			continue
		}
		ref, err := time.LoadLocation(iana)
		if err != nil {
			t.Fatal(err)
		}
		end := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
		for u := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); u.Before(end); u = u.Add(30 * time.Minute) {
			loc, ok := zoneAt(u, &tz)
			if !ok {
				t.Fatalf("%s: zoneAt(%s) failed", key, u)
			}
			_, got := u.In(loc).Zone()
			if _, want := u.In(ref).Zone(); got != want {
				t.Errorf("%s at %s UTC: offset %d, %s has %d", key, u.Format(time.DateTime), got, iana, want)
				break
			}
		}
	}
}

func findZone(key string) (dynamicTimeZoneInformation, bool) {
	for i := uintptr(0); ; i++ {
		var tz dynamicTimeZoneInformation
		r, _, _ := procEnumDynamicTimeZoneInformation.Call(i, uintptr(unsafe.Pointer(&tz)))
		if r != 0 {
			return tz, false
		}
		if windows.UTF16ToString(tz.TimeZoneKeyName[:]) == key {
			return tz, true
		}
	}
}
