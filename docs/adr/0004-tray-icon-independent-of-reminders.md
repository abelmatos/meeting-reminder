# 0004. Control the app from a tray icon that reminders don't depend on

- Status: Accepted
- Date: 2026-10-29

## Context

The app runs in the background with no window. Users need to see that it's
working, see the next meeting, change the calendar address, read the log and
exit. Windows can fail to add a tray icon (for example while Explorer is still
starting at login), and it notifies tray windows about shutdowns, including
ones that are later cancelled.

## Decision

- Provide a notification-area icon whose menu shows the next meeting and the
  app's actions.
- Run reminders independently of the icon: they start first and keep working
  if the icon is missing or removed. Only Exit, or a real session end, stops
  the app.
- Allow a single running instance. A second launch explains where the
  running instance is instead of starting another.

## Consequences

- A broken or missing tray icon costs controls, not reminders.
- After a cancelled shutdown the icon may disappear until the app is
  restarted, but reminders continue.
- Changing settings happens in the running instance (tray menu), not by
  launching the exe again with options.
