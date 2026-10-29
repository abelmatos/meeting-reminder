// Command reminder shows a topmost reminder card, beeping until dismissed, for each
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

func main() {
	setup := flag.Bool("setup", false, "replace the stored calendar URL")
	demo := flag.Bool("demo", false, "show a sample reminder and exit")
	card := flag.Bool("card", false, "internal: show one reminder card, reading the event from stdin")
	flag.Parse()

	if *card {
		os.Exit(runCard())
	}

	if closeLog, err := openLog(); err == nil {
		defer closeLog()
	}
	log.Printf("meeting-reminder %s starting, log file %q", version, logPath)

	if *demo {
		log.Print("showing a demo reminder")
		alert(demoEvent(), notScheduled)
		return
	}

	if !acquireSingleInstance() {
		log.Print("another instance is already running")
		showInfo("Meeting Reminder is already running.\n\n" +
			"Use the bell icon in the taskbar notification area to change the calendar or exit.")
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

// demoEvent is a sample meeting for previewing the reminder.
func demoEvent() Event {
	now := time.Now()
	return Event{
		ID:      "demo",
		Summary: "Demo meeting",
		Start:   now.Add(8 * time.Minute),
		End:     now.Add(38 * time.Minute),
		Link:    "https://meet.google.com/",
	}
}

// notScheduled is used for demo reminders, so a snoozed demo doesn't come back.
func notScheduled(string) bool { return false }

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
	log.Printf("calendar configured: %s in the next %s", plural(len(events), "event"), lookaheadText())
	showInfo(fmt.Sprintf("Setup complete. Found %s in the next %s.\n\n"+
		"You'll get a reminder %d minutes before each meeting. "+
		"The bell icon in the taskbar notification area has the app's options.",
		plural(len(events), "event"), lookaheadText(), int(leadTime.Minutes())))
	return feedURL, nil
}

// describeCalendar summarises a fetch for the log; it is logged only when it
// changes.
func describeCalendar(events []Event, now time.Time) string {
	upcoming := 0
	for _, e := range events {
		if !e.Start.Before(now) {
			upcoming++
		}
	}
	s := fmt.Sprintf("calendar: %s in the next %s", plural(upcoming, "event"), lookaheadText())
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
		return "No meetings in the next " + lookaheadText()
	}
	start := s.Next.Start.Local()
	day := ""
	if sameDay(start, now.AddDate(0, 0, 1)) {
		day = " tomorrow"
	}
	summary := s.Next.Summary
	if r := []rune(summary); len(r) > 40 {
		summary = string(r[:39]) + "…"
	}
	return fmt.Sprintf("Next: %s at %s%s", summary, start.Format("15:04"), day)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func lookaheadText() string {
	return plural(int(lookahead.Hours()), "hour")
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
	// A windowsgui process has no usable stderr; ignore its errors so they
	// don't cut writes short (e.g. copying a card's stderr into the log).
	log.SetOutput(io.MultiWriter(f, ignoreErrors{os.Stderr}))
	return func() { _ = f.Close() }, nil
}

// ignoreErrors is a writer that never fails.
type ignoreErrors struct{ w io.Writer }

func (i ignoreErrors) Write(p []byte) (int, error) {
	_, _ = i.w.Write(p)
	return len(p), nil
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

// formatAlert is the plain-text reminder, used where the card isn't.
func formatAlert(e Event, now time.Time) string {
	msg := fmt.Sprintf("%s\n\n%s · %s", e.Summary, startStatus(e.Start.Sub(now)), e.Start.Local().Format("15:04"))
	if e.Link != "" {
		msg += "\n\n" + e.Link
	}
	return msg
}
