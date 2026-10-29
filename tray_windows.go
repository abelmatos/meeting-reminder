//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/systray"
)

//go:embed winres/icon.ico
var trayIcon []byte

// runUI shows the notification-area icon and runs the reminder loop until
// the user picks Exit or ctx is cancelled.
func runUI(ctx context.Context, stop context.CancelFunc, src *ICalSource) {
	systray.Run(func() { onTrayReady(ctx, src) }, stop)
}

func onTrayReady(ctx context.Context, src *ICalSource) {
	systray.SetIcon(trayIcon)
	systray.SetTooltip(appTitle)

	mNext := systray.AddMenuItem("Loading calendar…", "Join the next meeting")
	mNext.Disable()
	systray.AddSeparator()
	mRefresh := systray.AddMenuItem("Refresh now", "Download the calendar again")
	mSetup := systray.AddMenuItem("Change calendar…", "Use a different secret iCal address")
	mLog := systray.AddMenuItem("Open log", "")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Stop meeting reminders")

	refresh := make(chan struct{}, 1)
	triggerRefresh := func() {
		select {
		case refresh <- struct{}{}:
		default:
		}
	}

	var mu sync.Mutex
	var shown, nextLink string
	go run(ctx, src, alert, refresh, func(s Status) {
		text := describeStatus(s, time.Now())
		link := ""
		if s.Next != nil {
			link = joinURL(s.Next.Link)
		}
		mu.Lock()
		defer mu.Unlock()
		nextLink = link
		if text == shown {
			return
		}
		shown = text
		mNext.SetTitle(text)
		systray.SetTooltip(appTitle + "\n" + text)
		if link != "" {
			mNext.Enable()
		} else {
			mNext.Disable()
		}
	})

	go func() {
		for {
			select {
			case <-mNext.ClickedCh:
				mu.Lock()
				link := nextLink
				mu.Unlock()
				if link != "" {
					logErr("open meeting link", openURL(link))
				}
			case <-mRefresh.ClickedCh:
				triggerRefresh()
			case <-mSetup.ClickedCh:
				u, err := configure(ctx, src.Client)
				switch {
				case errors.Is(err, errCancelled):
				case err != nil:
					log.Printf("setup failed: %v", err)
					showError(fmt.Sprintf("Setup failed: %v", err))
				default:
					src.SetURL(u)
					triggerRefresh()
				}
			case <-mLog.ClickedCh:
				if logPath != "" {
					logErr("open log", openURL(logPath))
				}
			case <-mExit.ClickedCh:
				systray.Quit()
				return
			case <-ctx.Done():
				systray.Quit()
				return
			}
		}
	}()
}

func logErr(what string, err error) {
	if err != nil {
		log.Printf("%s: %v", what, err)
	}
}
