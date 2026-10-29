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
  next meeting and has **Refresh now**, **Show demo reminder**,
  **Change calendar…**, **Open log** and **Exit**. Exit also closes any open
  reminder.
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
GitHub Actions, so no Windows toolchain is needed. Run every check for both
platforms, since the Windows-only files only compile with `GOOS=windows`:

```sh
go vet ./... && GOOS=windows go vet ./...
go test ./...                     # add -race for the concurrent code
golangci-lint run ./... && GOOS=windows golangci-lint run ./...   # as in CI

# Run on Linux/WSL: reminders are printed to stdout instead of shown as a card
MEETING_REMINDER_ICAL_URL='https://calendar.google.com/calendar/ical/.../basic.ics' go run .

# Cross-compile the Windows binary locally (CI names it reminder.exe)
GOOS=windows CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -H windowsgui" -o reminder.exe .
```

Everything is in `package main` at the repository root, except the feed
parser. The root must stay the main package: the Windows resources
(`rsrc_windows_amd64.syso`) and the embedded tray icon only work there.

| File | Purpose |
| --- | --- |
| `main.go` | Flags, logging, setup, wiring |
| `internal/calendar/` | Feed download and iCal parsing, recurrence expansion |
| `scheduler.go` | Fetch/check loop; starts reminders |
| `filter.go` | Which events are due, next event |
| `reminder.go` | Reminder loop: join, snooze, re-check, Exit (all platforms) |
| `reminder_windows.go` / `reminder_other.go` | Show a reminder once: card with beep and message-box fallback / stdout |
| `card.go`, `card.html` | The reminder card page |
| `cardproc_windows.go`, `cardproto.go` | Run a card in a child process; read its answer |
| `cardwindow_windows.go` | The card's borderless WebView2 window (child process) |
| `status.go` | Tray status, log summary and plain-text reminder wording |
| `tray_windows.go` / `tray_other.go` | Notification-area icon and menu / none |
| `dialog_windows.go` / `dialog_other.go` | Message boxes / stdout |
| `platform_windows.go` / `platform_other.go` | Credential Manager, clipboard, single instance, open links / env-var stub |
| `win32_windows.go` | Win32 DLLs, procedures and constants |
| `winres/`, `rsrc_windows_amd64.syso` | Icon (`icon.svg` source), manifest and version info, and their compiled form |

### Reminder card process

Each card runs as `reminder.exe -card` (see
[ADR 0003](docs/adr/0003-reminder-card-in-webview2-child-process.md)). The app
writes the event as JSON to the child's stdin. The child prints `shown` once
the card is visible, then the user's action (`join`, `snooze` or `dismiss`),
one per line, and logs to stderr, which the app copies into its log. If
`shown` doesn't arrive within 20 seconds, or the child exits without an
action, the app shows a message box instead.

### Windows resources

The manifest makes the app per-monitor DPI aware (sharp text on scaled
displays) and enables modern Windows control styles. After changing anything
in `winres/`, regenerate the resources with the pinned `go-winres` tool:

```sh
go generate
```

## Releases

Every push to `main` builds `reminder.exe` as a workflow artifact (kept 90
days). To publish a release with the exe and its SHA-256 checksum:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

## License

[MIT](LICENSE)
