// Command reminder shows a topmost popup, beeping until dismissed, for each
// Google Calendar meeting that starts within the next few minutes.
package main

import (
	"context"
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

func main() {
	setup := flag.Bool("setup", false, "replace the stored calendar URL")
	flag.Parse()

	if closeLog, err := openLog(); err == nil {
		defer closeLog()
	}
	log.Printf("meeting-reminder %s starting", version)

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
		if feedURL, err = configure(ctx, src); err != nil {
			log.Printf("setup failed: %v", err)
			showError(fmt.Sprintf("Setup failed: %v", err))
			return
		}
	}
	src.URL = feedURL

	run(ctx, src, alert)
	log.Print("stopped")
}

// configure asks the user for the feed URL, checks that it works, and stores it.
func configure(ctx context.Context, src *ICalSource) (string, error) {
	feedURL, err := promptFeedURL()
	if err != nil {
		return "", err
	}
	if err := ValidateFeedURL(feedURL); err != nil {
		return "", err
	}
	src.URL = feedURL
	now := time.Now()
	events, err := src.Events(ctx, now, now.Add(lookahead))
	if err != nil {
		return "", err
	}
	if err := saveFeedURL(feedURL); err != nil {
		return "", fmt.Errorf("store calendar URL: %w", err)
	}
	showInfo(fmt.Sprintf("Setup complete. Found %d events in the next 24 hours.\n\n"+
		"You'll get a popup %d minutes before each meeting.", len(events), int(leadTime.Minutes())))
	return feedURL, nil
}

// run refreshes events every fetchInterval and notifies about due events every
// checkInterval until ctx is cancelled. A failed fetch keeps the last events.
func run(ctx context.Context, src EventSource, notify func(Event)) {
	var events []Event
	var lastFetch time.Time
	seen := map[string]time.Time{}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		now := time.Now()
		if now.Sub(lastFetch) >= fetchInterval {
			evs, err := src.Events(ctx, now.Add(-startGrace), now.Add(lookahead))
			if err != nil {
				log.Printf("fetch failed: %v", err)
			} else {
				events, lastFetch = evs, now
			}
		}
		for _, e := range Due(events, now, leadTime, seen) {
			seen[e.ID] = e.Start
			log.Printf("alerting %q at %s", e.Summary, e.Start.Format(time.RFC3339))
			go notify(e)
		}
		PruneSeen(seen, now.Add(-time.Hour))

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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

func formatAlert(e Event, now time.Time) string {
	msg := fmt.Sprintf("%s\n\nStarts at %s", e.Summary, e.Start.Local().Format("15:04"))
	if mins := int(e.Start.Sub(now).Round(time.Minute).Minutes()); mins > 0 {
		msg += fmt.Sprintf(" (in %d min)", mins)
	} else {
		msg += " (now)"
	}
	if e.Link != "" {
		msg += "\n\n" + e.Link
	}
	return msg
}
