//go:build windows

package main

import (
	"log"
	"time"
)

// beepInterval is how often a reminder beeps while it is open.
const beepInterval = 30 * time.Second

// Actions a reminder card can return; card.html posts them.
const (
	actionJoin    = "join"
	actionSnooze  = "snooze"
	actionDismiss = "dismiss"
)

// alert shows the reminder card for e, beeping every beepInterval until the
// user acts on it. A snoozed reminder comes back after snoozeDuration while
// stillScheduled reports the meeting is still on and it hasn't ended.
func alert(e Event, stillScheduled func(id string) bool) {
	for {
		action := remind(e)
		log.Printf("reminder %q: %s", e.Summary, action)
		switch action {
		case actionJoin:
			if link := joinURL(e.Link); link != "" {
				if err := openURL(link); err != nil {
					log.Printf("open meeting link: %v", err)
				}
			}
			return
		case actionSnooze:
			time.Sleep(snoozeDuration)
			if time.Now().After(e.End) || !stillScheduled(e.ID) {
				log.Printf("reminder %q: not shown again, the meeting ended, moved or was cancelled", e.Summary)
				return
			}
		default:
			return
		}
	}
}

func remind(e Event) string {
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(beepInterval)
		defer t.Stop()
		for {
			_, _, _ = procMessageBeep.Call(mbIconWarning)
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()

	action, err := showCard(e)
	if err != nil {
		log.Printf("reminder card failed, using a message box: %v", err)
		messageBox(formatAlert(e, time.Now()), mbOK|mbIconWarning|mbSystemModal|mbSetForeground|mbTopmost)
		return actionDismiss
	}
	return action
}
