//go:build windows

package main

import (
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetLocalTime = kernel32.NewProc("GetLocalTime")

// The layout is fixed by winbase.h and GetDynamicTimeZoneInformation writes
// through it; a silent mismatch would corrupt every field.
func TestDynamicTimeZoneInformationLayout(t *testing.T) {
	if n := unsafe.Sizeof(dynamicTimeZoneInformation{}); n != 432 {
		t.Fatalf("sizeof(dynamicTimeZoneInformation) = %d, want 432", n)
	}
}

// localTime must agree with the clock the machine keeps right now, whatever
// zone it is in and however many times it changed since the process started.
func TestLocalTimeMatchesTheClockNow(t *testing.T) {
	var st windows.Systemtime
	r1, _, err := procGetLocalTime.Call(uintptr(unsafe.Pointer(&st)))
	if r1 == 0 {
		t.Skipf("GetLocalTime failed: %v", err)
	}
	want := naiveSeconds(int(st.Year), int(st.Month), int(st.Day), int(st.Hour), int(st.Minute), int(st.Second))

	got := localTime(time.Now())
	y, m, d := got.Date()
	have := naiveSeconds(y, int(m), d, got.Hour(), got.Minute(), got.Second())
	if diff := have - want; diff > 2 || diff < -2 {
		t.Fatalf("localTime(now) = %s, the clock says day %d month %d year %d %02d:%02d:%02d",
			got.Format(time.RFC3339), st.Day, st.Month, st.Year, st.Hour, st.Minute, st.Second)
	}
	if got.Location().String() == "" {
		t.Fatalf("localTime(now) carries no zone name: %s", got.Format(time.RFC3339))
	}
}

// A zone frozen on the bias it has now gets every date on the other side of a
// DST switch an hour off. Injected with the GMT Standard Time rules, the same
// converter must give each instant its own offset; a FixedZone built from one
// ActiveTimeBias cannot pass both halves of this test.
func TestConvertWithZoneAppliesEachDateDSTRule(t *testing.T) {
	dtzi := &dynamicTimeZoneInformation{
		Bias:            0,
		StandardName:    utf16Short("GMT Standard Time"),
		StandardDate:    windows.Systemtime{Month: 10, Day: 5, DayOfWeek: 0, Hour: 2}, // last Sunday, 02:00 daylight wall
		DaylightName:    utf16Short("GMT Daylight Time"),
		DaylightDate:    windows.Systemtime{Month: 3, Day: 5, DayOfWeek: 0, Hour: 1}, // last Sunday, 01:00 standard wall
		DaylightBias:    -60,
		TimeZoneKeyName: utf16Fixed("GMT Standard Time"),
	}

	cases := []struct {
		utc      time.Time
		wantOff  int
		wantHour int
		wantName string
	}{
		{time.Date(2027, 1, 15, 9, 0, 0, 0, time.UTC), 0, 9, "GMT Standard Time"},      // mid-winter
		{time.Date(2027, 7, 15, 9, 0, 0, 0, time.UTC), 3600, 10, "GMT Daylight Time"},  // mid-summer, the maintainer's repro
		{time.Date(2027, 3, 28, 0, 30, 0, 0, time.UTC), 0, 0, "GMT Standard Time"},     // half an hour before spring-forward
		{time.Date(2027, 3, 28, 2, 0, 0, 0, time.UTC), 3600, 3, "GMT Daylight Time"},   // right after spring-forward
		{time.Date(2027, 10, 31, 0, 30, 0, 0, time.UTC), 3600, 1, "GMT Daylight Time"}, // half an hour before fall-back
		{time.Date(2027, 10, 31, 1, 30, 0, 0, time.UTC), 0, 1, "GMT Standard Time"},    // right after fall-back
	}
	for _, c := range cases {
		got := convertWithZone(c.utc, dtzi)
		_, off := got.Zone()
		if off != c.wantOff {
			t.Errorf("%s: offset = %d, want %d (wall %s)", c.utc, off, c.wantOff, got.Format("15:04"))
			continue
		}
		if got.Hour() != c.wantHour {
			t.Errorf("%s: wall hour = %d, want %02d:00", c.utc, got.Hour(), c.wantHour)
		}
		if got.Location().String() != c.wantName {
			t.Errorf("%s: zone name = %q, want %q", c.utc, got.Location().String(), c.wantName)
		}
		if london, err := time.LoadLocation("Europe/London"); err == nil {
			if want := c.utc.In(london).Format("2006-01-02 15:04"); got.Format("2006-01-02 15:04") != want {
				t.Errorf("%s: wall = %s, tzdata Europe/London says %s", c.utc, got.Format("2006-01-02 15:04"), want)
			}
		}
	}
}

func naiveSeconds(year, month, day, hour, minute, second int) int {
	return ((((year*12+month)*31+day)*24+hour)*60+minute)*60 + second
}

func utf16Fixed(s string) [128]uint16 {
	var out [128]uint16
	u, _ := windows.UTF16FromString(s)
	copy(out[:], u)
	return out
}

func utf16Short(s string) [32]uint16 {
	var out [32]uint16
	u, _ := windows.UTF16FromString(s)
	copy(out[:], u)
	return out
}
