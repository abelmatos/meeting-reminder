package main

import (
	"context"
	"testing"
	"time"
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
