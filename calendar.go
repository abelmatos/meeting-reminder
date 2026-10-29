package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// maxFeedSize caps how much of the feed is read; a large calendar is ~1 MB.
const maxFeedSize = 32 << 20

// Event is one concrete occurrence of a calendar entry.
type Event struct {
	ID      string // unique per occurrence: UID plus actual start time
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
	Link    string // video call link, if any
}

// EventSource lists the events that start within [from, to].
type EventSource interface {
	Events(ctx context.Context, from, to time.Time) ([]Event, error)
}

// ICalSource reads events from an iCalendar feed, such as Google Calendar's
// "secret address in iCal format".
type ICalSource struct {
	Client *http.Client
	// Warn receives events that couldn't be parsed and were skipped.
	Warn func(error)

	mu  sync.Mutex
	url string
}

// SetURL changes the feed URL; safe to call while Events runs.
func (s *ICalSource) SetURL(u string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.url = u
}

func (s *ICalSource) feedURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *ICalSource) Events(ctx context.Context, from, to time.Time) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.feedURL(), nil)
	if err != nil {
		return nil, redact(err)
	}
	req.Header.Set("User-Agent", "meeting-reminder/"+version)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, redact(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch calendar: HTTP %s", resp.Status)
	}
	events, warnings, err := ParseEvents(io.LimitReader(resp.Body, maxFeedSize), from, to)
	if s.Warn != nil {
		for _, w := range warnings {
			s.Warn(w)
		}
	}
	return events, err
}

// redact strips the URL from HTTP client errors: the feed URL is a secret.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("fetch calendar: %s: %w", ue.Op, ue.Err)
	}
	return err
}

// ValidateFeedURL checks that s looks like an iCalendar feed URL.
func ValidateFeedURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return errors.New("not a valid URL")
	}
	if u.Scheme != "https" || u.Host == "" {
		return errors.New("must be an https:// URL")
	}
	if !strings.HasSuffix(u.Path, ".ics") {
		return errors.New("must end in .ics (use the \"Secret address in iCal format\")")
	}
	return nil
}

// ParseEvents decodes an iCalendar feed and returns the occurrences that start
// within [from, to], with recurring events expanded. Cancelled events and
// events the calendar owner declined are left out. Events that fail to parse
// are skipped and reported in warnings.
func ParseEvents(r io.Reader, from, to time.Time) (events []Event, warnings []error, err error) {
	cal, err := ical.NewDecoder(r).Decode()
	if err != nil {
		return nil, nil, fmt.Errorf("parse calendar: %w", err)
	}

	// Google names a user's primary calendar after their email address.
	owner, _ := cal.Props.Text("X-WR-CALNAME")
	owner = strings.ToLower(owner)

	// Overrides replace single occurrences of a recurring event, keyed by UID
	// and the original start of the occurrence they replace.
	var masters []ical.Event
	overridden := map[string]map[int64]bool{}
	for _, ev := range cal.Events() {
		rid := ev.Props.Get(ical.PropRecurrenceID)
		if rid == nil {
			masters = append(masters, ev)
			continue
		}
		uid, _ := ev.Props.Text(ical.PropUID)
		orig, err := rid.DateTime(time.Local)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("event %q: RECURRENCE-ID: %w", uid, err))
			continue
		}
		if overridden[uid] == nil {
			overridden[uid] = map[int64]bool{}
		}
		overridden[uid][orig.Unix()] = true
		events, warnings = appendOccurrences(events, warnings, ev, owner, from, to, nil)
	}
	for _, ev := range masters {
		uid, _ := ev.Props.Text(ical.PropUID)
		events, warnings = appendOccurrences(events, warnings, ev, owner, from, to, overridden[uid])
	}
	return events, warnings, nil
}

func appendOccurrences(events []Event, warnings []error, ev ical.Event, owner string, from, to time.Time, skip map[int64]bool) ([]Event, []error) {
	uid, _ := ev.Props.Text(ical.PropUID)
	if status, _ := ev.Status(); status == ical.EventCancelled || declinedBy(ev, owner) {
		return events, warnings
	}
	start, err := ev.DateTimeStart(time.Local)
	if err != nil {
		return events, append(warnings, fmt.Errorf("event %q: DTSTART: %w", uid, err))
	}
	end, err := ev.DateTimeEnd(time.Local)
	if err != nil {
		return events, append(warnings, fmt.Errorf("event %q: DTEND: %w", uid, err))
	}

	starts := []time.Time{start}
	if ev.Props.Get(ical.PropRecurrenceRule) != nil {
		set, err := recurrenceSet(ev, start)
		if err != nil {
			return events, append(warnings, fmt.Errorf("event %q: %w", uid, err))
		}
		starts = set.Between(from, to, true)
	}

	summary, _ := ev.Props.Text(ical.PropSummary)
	if summary = strings.TrimSpace(summary); summary == "" {
		summary = "(No title)"
	}
	allDay := isDate(ev.Props.Get(ical.PropDateTimeStart))
	link := conferenceLink(ev)
	for _, s := range starts {
		if s.Before(from) || s.After(to) || skip[s.Unix()] {
			continue
		}
		events = append(events, Event{
			ID:      uid + "/" + s.UTC().Format(time.RFC3339),
			Summary: summary,
			Start:   s,
			End:     s.Add(end.Sub(start)),
			AllDay:  allDay,
			Link:    link,
		})
	}
	return events, warnings
}

// recurrenceSet builds the RRULE/RDATE/EXDATE set of ev. It accepts
// comma-separated date lists, which go-ical's own RecurrenceSet does not.
func recurrenceSet(ev ical.Event, start time.Time) (*rrule.Set, error) {
	opt, err := ev.Props.RecurrenceRule()
	if err != nil {
		return nil, err
	}
	opt.Dtstart = start
	rule, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, fmt.Errorf("RRULE: %w", err)
	}
	set := &rrule.Set{}
	set.RRule(rule)
	set.DTStart(start)

	dates := func(name string) ([]time.Time, error) {
		var out []time.Time
		for _, prop := range ev.Props[name] {
			for _, v := range strings.Split(prop.Value, ",") {
				p := prop
				p.Value = v
				t, err := p.DateTime(time.Local)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
				out = append(out, t)
			}
		}
		return out, nil
	}
	exdates, err := dates(ical.PropExceptionDates)
	if err != nil {
		return nil, err
	}
	for _, t := range exdates {
		set.ExDate(t)
	}
	rdates, err := dates(ical.PropRecurrenceDates)
	if err != nil {
		return nil, err
	}
	for _, t := range rdates {
		set.RDate(t)
	}
	return set, nil
}

func declinedBy(ev ical.Event, owner string) bool {
	if owner == "" {
		return false
	}
	for _, a := range ev.Props[ical.PropAttendee] {
		email := strings.ToLower(strings.TrimPrefix(strings.ToLower(a.Value), "mailto:"))
		if email == owner && strings.EqualFold(a.Params.Get(ical.ParamParticipationStatus), "DECLINED") {
			return true
		}
	}
	return false
}

func isDate(prop *ical.Prop) bool {
	return prop != nil && (prop.ValueType() == ical.ValueDate || len(prop.Value) == len("20060102"))
}

func conferenceLink(ev ical.Event) string {
	if link, _ := ev.Props.Text("X-GOOGLE-CONFERENCE"); link != "" {
		return link
	}
	// A location may hold a URL followed by free text ("https://zoom.us/j/1, Room A").
	if loc, _ := ev.Props.Text(ical.PropLocation); strings.HasPrefix(loc, "https://") {
		if i := strings.IndexAny(loc, " \t\n,;"); i >= 0 {
			loc = loc[:i]
		}
		return loc
	}
	return ""
}
