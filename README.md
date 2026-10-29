# meeting-reminder

A small Windows app that watches your Google Calendar and shows a topmost
reminder card for every meeting starting in the next 10 minutes. It beeps
every 30 seconds until you **Join**, **Snooze** or dismiss it.

- No Google Cloud project or OAuth: it reads your calendar's private iCal feed.
- No installer: one `reminder.exe`. The card is a small HTML page shown with
  the WebView2 runtime that ships with Windows 10/11, with a live countdown,
  light and dark mode, and a **Join meeting** button for Meet, Zoom and Teams
  links. Without WebView2 it falls back to a plain message box.
- A bell icon in the notification area (next to volume and battery) shows the
  next meeting and has **Refresh now**, **Change calendar…**, **Open log** and
  **Exit**.
- Skips all-day events, blocks of 12 hours or more (e.g. "Out of office"),
  cancelled events and meetings you declined.
- Handles recurring meetings, moved or cancelled occurrences, and time zones.

## Setup

1. **Get the exe.** Download `reminder.exe` from the
   [latest release](../../releases/latest), or from the `reminder-exe`
   artifact of a [build run](../../actions/workflows/build.yml), and move it
   to a folder of its own (e.g. `%LOCALAPPDATA%\Programs\meeting-reminder`)
   rather than running it from Downloads.
   Windows SmartScreen may warn about an unsigned download: click
   **More info → Run anyway**.
2. **Copy your calendar's secret address.** In Google Calendar open
   **Settings → (your calendar) → Integrate calendar** and copy
   **Secret address in iCal format**. If the field is missing, your
   Google Workspace admin has disabled it.
3. **Run `reminder.exe`.** On first start it asks you to copy the address and
   click OK. It reads the address from the clipboard, checks that the feed
   works, stores it in Windows Credential Manager and clears the clipboard.
4. **Start it with Windows (optional).** Press `Win+R`, run `shell:startup`
   and put a shortcut to `reminder.exe` in that folder.

To change the address later, use **Change calendar…** in the tray menu. When
the app isn't running, `reminder.exe -setup` does the same; while it runs, a
second launch only points you to the tray icon. The stored entry is
`meeting-reminder/ical-url` in Credential Manager (Windows Credentials). To
see what a reminder looks like, run `reminder.exe -demo`.

Windows may hide new tray icons in the overflow menu (the `^` arrow). Drag the
bell onto the taskbar to keep it visible.

> The secret address gives read access to your calendar to anyone who has it.
> If it leaks, use **Reset** next to it in Google Calendar and enter the new
> address with **Change calendar…**.

## How it works

| Step | Interval |
| --- | --- |
| Download the feed (gzip, ~60 KB) and expand events from 12 h ago to 24 h ahead | 5 min |
| Check for meetings starting within 10 min (or started < 1 min ago) | 30 s |
| Beep while a reminder is open | 30 s |
| Show a snoozed reminder again (until the meeting ends, unless it was cancelled, declined or moved) | 5 min |

If a download fails, the app keeps using the last events it fetched. Each
occurrence alerts once; a meeting that is moved alerts again at its new time.
Only one instance runs at a time. Reminders keep working even if the tray
icon can't be added. Each card runs in a short-lived child process
(`reminder.exe -card`); if it fails or doesn't appear within 20 seconds, a
plain message box is shown instead. The log is at
`%APPDATA%\meeting-reminder\reminder.log`; it never contains the feed address.

Design decisions and their reasons are recorded in [docs/adr](docs/adr/).

## Development

Requires Go (see `go.mod`); everything else is optional. Windows builds run in
GitHub Actions, so no Windows toolchain is needed.

```sh
go vet ./... && GOOS=windows go vet ./...   # check both platforms
go test ./...
golangci-lint run                           # optional, same config as CI

# Run on Linux/WSL: alerts are printed to stdout instead of a popup
MEETING_REMINDER_ICAL_URL='https://calendar.google.com/calendar/ical/.../basic.ics' go run .

# Cross-compile the Windows binary locally
GOOS=windows CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -H windowsgui" -o reminder.exe .
```

| File | Purpose |
| --- | --- |
| `main.go` | Startup, setup, logging, status text |
| `scheduler.go` | Fetch/check loop, snooze check |
| `calendar.go` | Feed download and iCal parsing, recurrence expansion |
| `filter.go` | Which events are due, next event (pure logic) |
| `popup.go`, `popup.html` | Reminder card page (pure logic, testable anywhere) |
| `card_windows.go` | Runs each card in a child process, with timeout and fallback |
| `popup_windows.go` | Borderless topmost window hosting the card in WebView2 (child side) |
| `alert_windows.go` / `alert_other.go` | Beep, snooze/join handling, message boxes / stdout stub |
| `tray_windows.go` / `ui_other.go` | Notification-area icon and menu / plain loop |
| `platform_windows.go` / `platform_other.go` | Credential Manager, clipboard, single instance / env-var stub |
| `winres/` | Icon (`icon.svg` source, `icon.ico`), manifest and version info |
| `rsrc_windows_amd64.syso` | Those resources compiled for the linker; regenerate after editing `winres/` |

The manifest makes the app per-monitor DPI aware (sharp text on scaled
displays) and enables modern Windows control styles. After changing anything
in `winres/`, regenerate the resources:

```sh
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```

## Releases

Every push to `main` builds `reminder.exe` as a workflow artifact (kept 90
days). To publish a release with the exe and its SHA-256 checksum:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

## License

[MIT](LICENSE)
