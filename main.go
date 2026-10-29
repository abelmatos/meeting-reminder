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

func main() {
	setup := flag.Bool("setup", false, "replace the stored calendar URL")
	demo := flag.Bool("demo", false, "show a sample reminder and exit")
	card := flag.Bool("card", false, "internal: show one reminder card, reading the event from stdin")
	flag.Parse()

	if *card {
		os.Exit(runCard())
	}

	logPath, closeLog, err := openLog()
	if err == nil {
		defer closeLog()
	}
	log.Printf("meeting-reminder %s starting, log file %q", version, logPath)

	if *demo {
		log.Print("showing a demo reminder")
		alert(context.Background(), demoEvent(), notScheduled)
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

	// Reminders run independently of the tray icon (docs/adr/0004).
	rem := newReminders(ctx)
	t := newTray(src, logPath, rem)
	var sched *Scheduler
	sched = NewScheduler(src, func(e Event) { rem.start(e, sched.Scheduled) }, t.setStatus)
	go sched.Run(ctx)
	t.run(ctx, sched)

	stop() // close open cards and drop snoozed reminders
	rem.wait(shutdownTimeout)
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

// openLog sends the log to reminder.log in the user's config directory, since
// a windowsgui binary has no console. The previous log is kept once it grows.
func openLog() (path string, closeLog func(), err error) {
	dir, err := configDir()
	if err != nil {
		return "", nil, err
	}
	path = filepath.Join(dir, "reminder.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, //nolint:gosec // fixed name in the user config dir
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", nil, err
	}
	// A windowsgui process has no usable stderr; ignore its errors so they
	// don't cut writes short (e.g. copying a card's stderr into the log).
	log.SetOutput(io.MultiWriter(f, ignoreErrors{os.Stderr}))
	return path, func() { _ = f.Close() }, nil
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
