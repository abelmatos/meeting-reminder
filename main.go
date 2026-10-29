// Command reminder shows a topmost popup, beeping until dismissed, for each
// Google Calendar meeting that starts within the next few minutes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
	_ "time/tzdata" // Windows has no zoneinfo database; calendars use TZIDs like Europe/Zurich
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const (
	leadTime      = 10 * time.Minute
	checkInterval = 30 * time.Second
	fetchInterval = 5 * time.Minute
	lookahead     = 24 * time.Hour
	maxLogSize    = 1 << 20
)

var errCancelled = errors.New("cancelled")

// logPath is where openLog sends the log, if it succeeded.
var logPath string

// Status is what the app knows about the calendar, shown in the tray.
type Status struct {
	Next *Event // next meeting that will alert, if any
	Err  error  // last fetch error; nil once a fetch succeeds
}

func main() {
	setup := flag.Bool("setup", false, "replace the stored calendar URL")
	demo := flag.Bool("demo", false, "show a sample reminder and exit")
	flag.Parse()

	if closeLog, err := openLog(); err == nil {
		defer closeLog()
	}
	log.Printf("meeting-reminder %s starting, log file %q", version, logPath)

	if *demo {
		log.Print("showing a demo reminder")
		now := time.Now()
		alert(Event{
			ID:      "demo",
			Summary: "Demo meeting",
			Start:   now.Add(8 * time.Minute),
			End:     now.Add(38 * time.Minute),
			Link:    "https://meet.google.com/",
		})
		return
	}

	if !acquireSingleInstance() {
		log.Print("another instance is already running")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	src := &ICalSource{
		Client: &http.Client{Timeout: 30 * time.Second},
		Warn:   func(err error) { log.Printf("skipped event: %v", err) },
	}
	feedURL, err := loadFeedURL()
	if err != nil || *setup {
		if err != nil {
			log.Printf("no stored calendar URL: %v", err)
		}
		if feedURL, err = configure(ctx, src.Client); err != nil {
			log.Printf("setup failed: %v", err)
			if !errors.Is(err, errCancelled) {
				showError(fmt.Sprintf("Setup failed: %v", err))
			}
			return
		}
	}
	src.SetURL(feedURL)

	runUI(ctx, stop, src)
	log.Print("stopped")
}

// configure asks the user for the feed URL, checks that it works, and stores it.
func configure(ctx context.Context, client *http.Client) (string, error) {
	feedURL, err := promptFeedURL()
	if err != nil {
		return "", err
	}
	if err := ValidateFeedURL(feedURL); err != nil {
		return "", err
	}
	src := &ICalSource{Client: client}
	src.SetURL(feedURL)
	now := time.Now()
	events, err := src.Events(ctx, now, now.Add(lookahead))
	if err != nil {
		return "", err
	}
	if err := saveFeedURL(feedURL); err != nil {
		return "", fmt.Errorf("store calendar URL: %w", err)
	}
	log.Printf("calendar configured: %s in the next 24h", plural(len(events), "event"))
	showInfo(fmt.Sprintf("Setup complete. Found %s in the next 24 hours.\n\n"+
		"You'll get a reminder %d minutes before each meeting. "+
		"The bell icon in the taskbar notification area has the app's options.",
		plural(len(events), "event"), int(leadTime.Minutes())))
	return feedURL, nil
}

// run refreshes events every fetchInterval (or when refresh fires) and
// notifies about due events every checkInterval until ctx is cancelled. A
// failed fetch keeps the last events. onStatus, if set, gets the state after
// every check.
func run(ctx context.Context, src EventSource, notify func(Event), refresh <-chan struct{}, onStatus func(Status)) {
	var events []Event
	var lastFetch time.Time
	var fetchErr error
	var failures int
	var logged string // last calendar summary written to the log
	seen := map[string]time.Time{}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		now := time.Now()
		if now.Sub(lastFetch) >= fetchInterval {
			evs, err := src.Events(ctx, now.Add(-startGrace), now.Add(lookahead))
			if fetchErr = err; err != nil {
				// Failed fetches retry every check; log the first and then
				// every 20th (~10 minutes) so an outage doesn't flood the log.
				if failures++; failures == 1 || failures%20 == 0 {
					log.Printf("fetch failed (%s in a row): %v", plural(failures, "attempt"), err)
				}
			} else {
				if failures > 0 {
					log.Printf("fetch recovered after %s", plural(failures, "failed attempt"))
				}
				failures = 0
				events, lastFetch = evs, now
				if s := describeCalendar(events, now); s != logged {
					log.Print(s)
					logged = s
				}
			}
		}
		for _, e := range Due(events, now, leadTime, seen) {
			seen[e.ID] = e.Start
			log.Printf("alerting %q at %s", e.Summary, e.Start.Format(time.RFC3339))
			go notify(e)
		}
		PruneSeen(seen, now.Add(-time.Hour))
		if onStatus != nil {
			onStatus(Status{Next: NextEvent(events, now), Err: fetchErr})
		}

		select {
		case <-ctx.Done():
			return
		case <-refresh:
			lastFetch = time.Time{}
		case <-ticker.C:
		}
	}
}

// describeCalendar summarises a fetch for the log; it is logged only when it
// changes.
func describeCalendar(events []Event, now time.Time) string {
	s := fmt.Sprintf("calendar: %s in the next 24h", plural(len(events), "event"))
	if next := NextEvent(events, now); next != nil {
		s += fmt.Sprintf(", next %q at %s", next.Summary, next.Start.Local().Format("Mon 15:04"))
	}
	return s
}

// describeStatus is the one-line summary shown in the tray.
func describeStatus(s Status, now time.Time) string {
	if s.Next == nil {
		if s.Err != nil {
			return "Can't reach the calendar, retrying"
		}
		return "No meetings in the next 24 hours"
	}
	start := s.Next.Start.Local()
	day := ""
	if y, m, d := start.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
		day = " tomorrow"
	}
	summary := s.Next.Summary
	if r := []rune(summary); len(r) > 40 {
		summary = string(r[:39]) + "…"
	}
	return fmt.Sprintf("Next: %s at %s%s", summary, start.Format("15:04"), day)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// openLog sends the log to reminder.log in the user's config directory, since
// a windowsgui binary has no console. The previous log is kept once it grows.
func openLog() (func(), error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "reminder.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, //nolint:gosec // fixed name in the user config dir
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	logPath = path
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	return func() { _ = f.Close() }, nil
}

// configDir returns %APPDATA%\meeting-reminder (or the XDG equivalent).
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "meeting-reminder")
	return dir, os.MkdirAll(dir, 0o700)
}

// formatAlert is the plain-text reminder, used where the popup card isn't.
func formatAlert(e Event, now time.Time) string {
	msg := fmt.Sprintf("%s\n\n%s · %s", e.Summary, startStatus(e.Start.Sub(now)), e.Start.Local().Format("15:04"))
	if e.Link != "" {
		msg += "\n\n" + e.Link
	}
	return msg
}
