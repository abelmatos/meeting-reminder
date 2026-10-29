# Architecture decision records

Short records of the decisions that shape this app: what was decided, why,
and what it costs. Implementation details belong in the code and README.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-read-calendar-from-secret-ical-feed.md) | Read the calendar from the secret iCal feed | Accepted |
| [0002](0002-build-windows-binary-in-ci.md) | Build the Windows binary only in CI, in pure Go | Accepted |
| [0003](0003-reminder-card-in-webview2-child-process.md) | Show reminders as a WebView2 card in a child process | Accepted |
| [0004](0004-tray-icon-independent-of-reminders.md) | Control the app from a tray icon that reminders don't depend on | Accepted |
| [0005](0005-store-feed-url-in-credential-manager.md) | Store the feed URL in Windows Credential Manager | Accepted |

## Adding a record

Copy the structure of an existing record (Context, Decision, Consequences),
number it sequentially, and add it to the table. Don't edit an accepted
record's decision; supersede it with a new record and update both statuses.
