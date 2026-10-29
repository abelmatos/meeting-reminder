# 0003. Show reminders as a WebView2 card in a child process

- Status: Accepted
- Date: 2026-10-29

## Context

A reminder has to be hard to miss and quick to act on. A plain message box
works but looks dated and can't offer Join or Snooze. Windows toast
notifications look native, but they appear in a corner and can be hidden by
Do Not Disturb. A custom window gives full control over the look.

The WebView2 runtime ships with Windows 10 and 11, so a small HTML page can be
the card. The only pure-Go WebView2 binding available ends the process on some
errors, can hang during start-up, and never releases the browser it creates.

## Decision

- Render the reminder as an HTML card in a borderless, topmost WebView2
  window, with a countdown and Join, Snooze and OK actions.
- Run each card in a short-lived child process of the same exe. The main app
  only waits for the user's choice.
- If the child fails, exits without an answer, or doesn't show the card in
  time, show a plain message box instead. A reminder is never lost.

## Consequences

- WebView2 crashes, hangs and leaks are confined to the card's own process,
  and the browser is freed when it exits.
- The card's design is ordinary HTML and CSS, and can be previewed in a
  browser.
- Each reminder starts a process and a browser instance, which takes about a
  second. That is fine for a few reminders a day.
- The child loads DLLs only from System32, so files next to the exe can't be
  injected into it.
