# meeting-reminder

A small Windows app that watches your Google Calendar and shows a topmost
popup for every meeting starting in the next 10 minutes. It beeps every
30 seconds until you click **OK**.

- No Google Cloud project or OAuth: it reads your calendar's private iCal feed.
- No installer, no GUI framework: one `reminder.exe` using the native
  `MessageBoxW` dialog.
- Skips all-day events, blocks of 12 hours or more (e.g. "Out of office"),
  cancelled events and meetings you declined.
- Handles recurring meetings, moved or cancelled occurrences, and time zones.

## Setup

1. **Get the exe.** Download `reminder.exe` from the
   [latest release](../../releases/latest), or from the `reminder-exe`
   artifact of a [build run](../../actions/workflows/build.yml).
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

To change the address later, run `reminder.exe -setup`. The stored entry is
`meeting-reminder/ical-url` in Credential Manager (Windows Credentials).

> The secret address gives read access to your calendar to anyone who has it.
> If it leaks, use **Reset** next to it in Google Calendar and run
> `reminder.exe -setup` again.

## How it works

| Step | Interval |
| --- | --- |
| Download the feed (gzip, ~60 KB) and expand events for the next 24 h | 5 min |
| Check for meetings starting within 10 min (or started < 1 min ago) | 30 s |
| Beep while a popup is open | 30 s |

If a download fails, the app keeps using the last events it fetched. Each
occurrence alerts once; a meeting that is moved alerts again at its new time.
Only one instance runs at a time. The log is at
`%APPDATA%\meeting-reminder\reminder.log`; it never contains the feed address.

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
| `main.go` | Setup, fetch/check loop, logging |
| `calendar.go` | Feed download and iCal parsing, recurrence expansion |
| `filter.go` | Which events are due (pure logic) |
| `alert_windows.go` / `alert_other.go` | Popup and beep / stdout stub |
| `platform_windows.go` / `platform_other.go` | Credential Manager, clipboard, single instance / env-var stub |

## Releases

Every push to `main` builds `reminder.exe` as a workflow artifact (kept 90
days). To publish a release with the exe and its SHA-256 checksum:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

## License

[MIT](LICENSE)
