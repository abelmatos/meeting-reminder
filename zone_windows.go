//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dynamicTimeZoneInformation mirrors DYNAMIC_TIME_ZONE_INFORMATION from
// timezoneapi.h. TimeZoneKeyName comes before the one-byte BOOLEAN
// DynamicDaylightTimeDisabled; the test pins every offset, because a swapped
// pair still adds up to the same 432 bytes.
type dynamicTimeZoneInformation struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                windows.Systemtime
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                windows.Systemtime
	DaylightBias                int32
	TimeZoneKeyName             [128]uint16
	DynamicDaylightTimeDisabled uint8
}

// localTime converts an instant to the machine's wall time as of this call
// (#7). Go reads the zone once at startup, so after a Windows time-zone
// change time.Local keeps the old one until restart. Reading the current zone
// on every call follows the change, and converting through
// SystemTimeToTzSpecificLocalTimeEx applies the zone's rules for that date, so
// times on the other side of a daylight-saving switch get their own offset.
// On any failure it falls back to time.Local.
func localTime(t time.Time) time.Time {
	var tz dynamicTimeZoneInformation
	if err := getDynamicTimeZoneInformation(&tz); err != nil {
		return t.In(time.Local)
	}
	if loc, ok := zoneAt(t, &tz); ok {
		return t.In(loc)
	}
	return t.In(time.Local)
}

// zoneAt returns the fixed zone that tz's rules give instant t.
func zoneAt(t time.Time, tz *dynamicTimeZoneInformation) (*time.Location, bool) {
	utcFT := windows.NsecToFiletime(t.UnixNano())
	var utc, local windows.Systemtime
	if fileTimeToSystemTime(&utcFT, &utc) != nil ||
		systemTimeToTzSpecificLocalTimeEx(tz, &utc, &local) != nil {
		return nil, false
	}
	var localFT windows.Filetime
	if systemTimeToFileTime(&local, &localFT) != nil {
		return nil, false
	}
	offset := int((localFT.Nanoseconds() - utcFT.Nanoseconds()) / int64(time.Second))
	return time.FixedZone(zoneName(tz, offset), offset), true
}

// zoneName picks the standard or daylight name that matches offset.
func zoneName(tz *dynamicTimeZoneInformation, offset int) string {
	if offset == -int(tz.Bias+tz.DaylightBias)*60 && offset != -int(tz.Bias+tz.StandardBias)*60 {
		return windows.UTF16ToString(tz.DaylightName[:])
	}
	return windows.UTF16ToString(tz.StandardName[:])
}

// getDynamicTimeZoneInformation reads the zone the clock is in right now.
func getDynamicTimeZoneInformation(tz *dynamicTimeZoneInformation) error {
	r, _, err := procGetDynamicTimeZoneInformation.Call(uintptr(unsafe.Pointer(tz)))
	if r == timeZoneIDInvalid {
		return err
	}
	return nil
}

func systemTimeToTzSpecificLocalTimeEx(tz *dynamicTimeZoneInformation, utc, local *windows.Systemtime) error {
	r, _, err := procSystemTimeToTzSpecificLocalTimeEx.Call(
		uintptr(unsafe.Pointer(tz)),
		uintptr(unsafe.Pointer(utc)),
		uintptr(unsafe.Pointer(local)),
	)
	if r == 0 {
		return err
	}
	return nil
}

func fileTimeToSystemTime(ft *windows.Filetime, st *windows.Systemtime) error {
	r, _, err := procFileTimeToSystemTime.Call(uintptr(unsafe.Pointer(ft)), uintptr(unsafe.Pointer(st)))
	if r == 0 {
		return err
	}
	return nil
}

func systemTimeToFileTime(st *windows.Systemtime, ft *windows.Filetime) error {
	r, _, err := procSystemTimeToFileTime.Call(uintptr(unsafe.Pointer(st)), uintptr(unsafe.Pointer(ft)))
	if r == 0 {
		return err
	}
	return nil
}
