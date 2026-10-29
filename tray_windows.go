//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

//go:embed winres/icon.ico
var trayIcon []byte

// tray is the notification-area icon. Reminders don't depend on it: the
// scheduler starts first, so they keep working if the icon can't be added.
type tray struct {
	ctx   context.Context
	src   *ICalSource
	sched *Scheduler

	mu       sync.Mutex
	next     *systray.MenuItem // nil until the menu exists
	status   Status
	shown    string // label currently on the next-meeting item
	nextLink string

	configuring atomic.Bool
}

// runUI runs reminders and the tray icon until the user picks Exit or ctx is
// cancelled.
func runUI(ctx context.Context, _ context.CancelFunc, src *ICalSource) {
	t := &tray{ctx: ctx, src: src}
	t.sched = NewScheduler(src, func(e Event) { alert(e, t.sched.Scheduled) }, t.setStatus)
	go t.sched.Run(ctx)
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()
	// onExit also runs on WM_ENDSESSION even when the shutdown is cancelled,
	// so it must not stop the app; Exit and real session ends stop it anyway.
	systray.Run(t.onReady, func() { log.Print("tray icon removed") })
}

func (t *tray) onReady() {
	systray.SetIcon(trayIcon)
	systray.SetTooltip(appTitle)

	next := systray.AddMenuItem("Loading calendar…", "Join the next meeting")
	next.Disable()
	systray.AddSeparator()
	mRefresh := systray.AddMenuItem("Refresh now", "Download the calendar again")
	mDemo := systray.AddMenuItem("Show demo reminder", "Preview what a reminder looks like")
	mDemoNoGPU := systray.AddMenuItem("Show demo reminder (no GPU)", "Troubleshooting: preview a reminder rendered without the graphics card")
	mSetup := systray.AddMenuItem("Change calendar…", "Use a different secret iCal address")
	mLog := systray.AddMenuItem("Open log", "")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Stop meeting reminders")

	t.mu.Lock()
	t.next = next
	t.mu.Unlock()
	t.apply()

	go func() {
		for {
			select {
			case <-next.ClickedCh:
				t.mu.Lock()
				link := t.nextLink
				t.mu.Unlock()
				if link != "" {
					logErr("open meeting link", openURL(link))
				}
			case <-mRefresh.ClickedCh:
				log.Print("tray: refresh")
				t.sched.Refresh()
			case <-mDemo.ClickedCh:
				log.Print("tray: demo reminder")
				go alert(demoEvent(), notScheduled)
			case <-mDemoNoGPU.ClickedCh:
				log.Print("tray: demo reminder without GPU")
				go alertWith(demoEvent(), notScheduled, cardOptions{noGPU: true})
			case <-mSetup.ClickedCh:
				log.Print("tray: change calendar")
				// In its own goroutine: systray drops clicks nobody is
				// receiving, so this loop must stay free while dialogs are open.
				if t.configuring.CompareAndSwap(false, true) {
					go t.changeCalendar()
				}
			case <-mLog.ClickedCh:
				if logPath != "" {
					logErr("open log", openURL(logPath))
				}
			case <-mExit.ClickedCh:
				log.Print("tray: exit")
				systray.Quit()
				return
			}
		}
	}()
}

func (t *tray) changeCalendar() {
	defer t.configuring.Store(false)
	u, err := configure(t.ctx, t.src.Client)
	switch {
	case errors.Is(err, errCancelled):
		log.Print("setup cancelled")
	case err != nil:
		log.Printf("setup failed: %v", err)
		showError(fmt.Sprintf("Setup failed: %v", err))
	default:
		t.src.SetURL(u)
		t.sched.Refresh()
	}
}

// setStatus is the scheduler's status callback; it may run before the menu
// exists.
func (t *tray) setStatus(s Status) {
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
	t.apply()
}

func (t *tray) apply() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.next == nil {
		return
	}
	t.nextLink = ""
	if t.status.Next != nil {
		t.nextLink = joinURL(t.status.Next.Link)
	}
	// The link can change while the label stays the same.
	if t.nextLink != "" {
		t.next.Enable()
	} else {
		t.next.Disable()
	}
	text := describeStatus(t.status, time.Now())
	if text == t.shown {
		return
	}
	t.shown = text
	// "&" marks a keyboard shortcut in Win32 menus; "&&" shows one "&".
	t.next.SetTitle(strings.ReplaceAll(text, "&", "&&"))
	systray.SetTooltip(appTitle + "\n" + text)
}

func logErr(what string, err error) {
	if err != nil {
		log.Printf("%s: %v", what, err)
	}
}
