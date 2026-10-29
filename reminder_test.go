package main

import (
	"context"
	"testing"
	"time"

	"github.com/abelmatos/meeting-reminder/internal/calendar"
)

func TestRemindersWaitReturnsWhenRemindersFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := newReminders(ctx)
	r.start(demoEvent(), notScheduled)
	cancel()
	if !r.wait(time.Second) {
		t.Fatal("wait timed out although the reminder had finished")
	}
}

// fakeUI scripts the actions a reminder returns and records what it did.
type fakeUI struct {
	actions []string
	shows   int
	opened  []string
}

func (f *fakeUI) ui(now time.Time) reminderUI {
	return reminderUI{
		show: func(context.Context, calendar.Event) string {
			f.shows++
			if len(f.actions) == 0 {
				return actionDismiss
			}
			a := f.actions[0]
			f.actions = f.actions[1:]
			return a
		},
		open: func(link string) error {
			f.opened = append(f.opened, link)
			return nil
		},
		snooze: time.Millisecond,
		now:    func() time.Time { return now },
	}
}

func TestReminderLoop(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	meeting := calendar.Event{ID: "m", Summary: "Sync", Start: now.Add(5 * time.Minute), End: now.Add(35 * time.Minute), Link: "https://meet.google.com/abc"}
	scheduled := func(string) bool { return true }

	tests := []struct {
		name      string
		event     calendar.Event
		actions   []string
		scheduled func(string) bool
		now       time.Time
		wantShows int
		wantOpen  []string
	}{
		{"dismiss shows once", meeting, []string{actionDismiss}, scheduled, now, 1, nil},
		{"join opens the link", meeting, []string{actionJoin}, scheduled, now, 1, []string{"https://meet.google.com/abc"}},
		{"snooze shows again", meeting, []string{actionSnooze, actionSnooze, actionJoin}, scheduled, now, 3, []string{"https://meet.google.com/abc"}},
		{"snooze stops when the meeting is gone", meeting, []string{actionSnooze, actionJoin}, notScheduled, now, 1, nil},
		{"snooze stops after the meeting ended", meeting, []string{actionSnooze, actionJoin}, scheduled, meeting.End.Add(time.Second), 1, nil},
		{"join without an https link opens nothing", calendar.Event{ID: "m", End: meeting.End, Link: "javascript:alert(1)"}, []string{actionJoin}, scheduled, now, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeUI{actions: tt.actions}
			f.ui(tt.now).run(context.Background(), tt.event, tt.scheduled)
			if f.shows != tt.wantShows {
				t.Errorf("shown %d times, want %d", f.shows, tt.wantShows)
			}
			if len(f.opened) != len(tt.wantOpen) || (len(f.opened) > 0 && f.opened[0] != tt.wantOpen[0]) {
				t.Errorf("opened %v, want %v", f.opened, tt.wantOpen)
			}
		})
	}
}

func TestReminderLoopStopsOnCancel(t *testing.T) {
	now := time.Now()
	meeting := calendar.Event{ID: "m", End: now.Add(time.Hour)}
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeUI{actions: []string{actionSnooze, actionSnooze}}
	ui := f.ui(now)
	ui.snooze = time.Hour
	done := make(chan struct{})
	go func() {
		ui.run(ctx, meeting, func(string) bool { return true })
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("snoozed reminder did not stop when the context was cancelled")
	}
	if f.shows != 1 {
		t.Errorf("shown %d times, want 1", f.shows)
	}
}
