//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dynamicTimeZoneInformation mirrors DYNAMIC_TIME_ZONE_INFORMATION. x/sys
// carries only the plain TZI and GetTimeZoneInformation, so the two dynamic
// calls below go through LazyProc; the layout is fixed by winbase.h and the
// test asserts its size.
type dynamicTimeZoneInformation struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                windows.Systemtime
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                windows.Systemtime
	DaylightBias                int32
	DynamicDaylightTimeDisabled uint32
	TimeZoneKeyName             [128]uint16
}

// localTime converts an instant to the machine's current wall time, applying
// the date's own daylight-saving rule (#7). Go loads time.Local once at
// startup, so after a time-zone change the card and tray kept showing the old
// zone's wall times until restart; and a zone frozen on one bias gets every
// date on the wrong side of the next DST switch an hour off. Re-reading the
// dynamic zone information on every call follows a zone change without a
// restart, and SystemTimeToTzSpecificLocalTimeEx applies each date's own
// transition rules.
func localTime(t time.Time) time.Time {
	var dtzi dynamicTimeZoneInformation
	if err := getDynamicTimeZoneInformation(&dtzi); err != nil {
		return t.In(time.Local)
	}
	return convertWithZone(t, &dtzi)
}

// convertWithZone applies one zone's rules to one instant. It is a separate
// function so tests can inject a zone whose DST differs from the machine's.
func convertWithZone(t time.Time, dtzi *dynamicTimeZoneInformation) time.Time {
	utc, ok := systemTimeOf(t.UTC())
	if !ok {
		return t.In(time.Local)
	}
	var local windows.Systemtime
	if err := systemTimeToTzSpecificLocalTimeEx(dtzi, &utc, &local); err == nil {
		off := naiveSecondsBetween(&utc, &local)
		return time.Date(
			int(local.Year), time.Month(local.Month), int(local.Day),
			int(local.Hour), int(local.Minute), int(local.Second),
			int(local.Milliseconds)*1e6,
			time.FixedZone(zoneNameAt(dtzi, off), off),
		)
	}
	// SystemTimeToTzSpecificLocalTimeEx refuses zones whose per-year "Dynamic
	// DST" registry data is absent (ERROR_FILE_NOT_FOUND); evaluate the static
	// rules the structure itself carries instead.
	off, name := offsetForInstant(t.UTC(), dtzi)
	return t.In(time.FixedZone(name, off))
}

// getDynamicTimeZoneInformation reads the zone the machine's clock is in
// right now, rules and all. TIME_ZONE_ID_UNKNOWN (0) is a success value: it
// means the zone has no daylight saving this year.
func getDynamicTimeZoneInformation(dtzi *dynamicTimeZoneInformation) error {
	r1, _, err := procGetDynamicTimeZoneInformation.Call(uintptr(unsafe.Pointer(dtzi)))
	if r1 == timeZoneIDInvalid {
		return err
	}
	return nil
}

const timeZoneIDInvalid = 0xFFFFFFFF

// offsetForInstant applies the standard/daylight rules the zone structure
// carries to one instant, southern-hemisphere spans included.
func offsetForInstant(utc time.Time, dtzi *dynamicTimeZoneInformation) (int, string) {
	stdOff := -(int(dtzi.Bias) + int(dtzi.StandardBias)) * 60
	stdName := windows.UTF16ToString(dtzi.StandardName[:])
	if dtzi.StandardDate.Month == 0 || dtzi.DynamicDaylightTimeDisabled != 0 {
		return stdOff, stdName
	}
	// DST starts on a standard-wall clock and ends on a daylight-wall one.
	start := ruleUTC(utc.Year(), dtzi.DaylightDate, int(dtzi.Bias)+int(dtzi.StandardBias))
	end := ruleUTC(utc.Year(), dtzi.StandardDate, int(dtzi.Bias)+int(dtzi.DaylightBias))
	inDST := (start.Before(end) && !utc.Before(start) && utc.Before(end)) ||
		(start.After(end) && (!utc.Before(start) || utc.Before(end)))
	if inDST {
		return -(int(dtzi.Bias) + int(dtzi.DaylightBias)) * 60, windows.UTF16ToString(dtzi.DaylightName[:])
	}
	return stdOff, stdName
}

// ruleUTC resolves one transition rule for year: the nth (Day 1..4) or last
// (Day 5) DayOfWeek of Month at the rule's wall-clock hour, with wallMin the
// minutes east of UTC of the wall the rule is expressed in.
func ruleUTC(year int, st windows.Systemtime, wallMin int) time.Time {
	first := time.Date(year, time.Month(st.Month), 1, 0, 0, 0, 0, time.UTC)
	day := 1 + (int(st.DayOfWeek)-int(first.Weekday())+7)%7 + (int(st.Day)-1)*7
	if st.Day >= 5 {
		last := time.Date(year, time.Month(st.Month)+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
		day = last.Day() - (int(last.Weekday())-int(st.DayOfWeek)+7)%7
	}
	wall := time.Date(year, time.Month(st.Month), day, int(st.Hour), int(st.Minute), int(st.Second), 0, time.UTC)
	return wall.Add(time.Duration(wallMin) * time.Minute)
}

// systemTimeToTzSpecificLocalTimeEx converts one UTC instant into the wall
// time lpTimeZoneInformation's rules give that date, dynamic DST included.
func systemTimeToTzSpecificLocalTimeEx(dtzi *dynamicTimeZoneInformation, utc, local *windows.Systemtime) error {
	r1, _, err := procSystemTimeToTzSpecificLocalTimeEx.Call(
		uintptr(unsafe.Pointer(dtzi)),
		uintptr(unsafe.Pointer(utc)),
		uintptr(unsafe.Pointer(local)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

// systemTimeOf packs an instant into a SYSTEMTIME. It fails for instants
// SYSTEMTIME cannot carry and lets the caller fall back to time.Local.
func systemTimeOf(t time.Time) (windows.Systemtime, bool) {
	if t.IsZero() || t.Year() < 1601 || t.Year() > 30827 {
		return windows.Systemtime{}, false
	}
	u := t.UTC()
	return windows.Systemtime{
		Year:         uint16(u.Year()),
		Month:        uint16(u.Month()),
		Day:          uint16(u.Day()),
		Hour:         uint16(u.Hour()),
		Minute:       uint16(u.Minute()),
		Second:       uint16(u.Second()),
		Milliseconds: uint16(u.Nanosecond() / 1e6),
	}, true
}

// naiveSecondsBetween reports local minus UTC as wall-clock seconds: the
// offset this zone gives the instant. For the ambiguous hour after a fall-back
// the conversion lands on standard time, and this is its offset.
func naiveSecondsBetween(utc, local *windows.Systemtime) int {
	a := time.Date(int(utc.Year), time.Month(utc.Month), int(utc.Day),
		int(utc.Hour), int(utc.Minute), int(utc.Second), 0, time.UTC)
	b := time.Date(int(local.Year), time.Month(local.Month), int(local.Day),
		int(local.Hour), int(local.Minute), int(local.Second), 0, time.UTC)
	return int(b.Sub(a).Seconds())
}

// zoneNameAt picks the name that matches the offset the conversion chose.
func zoneNameAt(dtzi *dynamicTimeZoneInformation, off int) string {
	if off == -(int(dtzi.Bias)+int(dtzi.DaylightBias))*60 {
		if name := windows.UTF16ToString(dtzi.DaylightName[:]); name != "" {
			return name
		}
	}
	if off == -(int(dtzi.Bias)+int(dtzi.StandardBias))*60 {
		if name := windows.UTF16ToString(dtzi.StandardName[:]); name != "" {
			return name
		}
	}
	return windows.UTF16ToString(dtzi.TimeZoneKeyName[:])
}
