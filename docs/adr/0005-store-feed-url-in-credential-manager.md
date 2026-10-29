# 0005. Store the feed URL in Windows Credential Manager

- Status: Accepted
- Date: 2026-10-29

## Context

The secret iCal address ([0001](0001-read-calendar-from-secret-ical-feed.md))
grants read access to the whole calendar. It has to survive restarts, and the
user has to enter it once. The app has no text-entry UI.

## Decision

- Store the address in Windows Credential Manager, which encrypts it for the
  current Windows user.
- Take it from the clipboard during setup: the user copies it in Google
  Calendar and confirms. The app checks that the feed works before saving it,
  then clears the clipboard.
- Never write the address to the log or to error messages.

## Consequences

- No plain-text config file holds the secret, and other Windows users on the
  machine can't read it.
- Setup needs no input form, but relies on the user copying the right text.
  The address is validated and test-fetched before it is accepted.
- Moving to another PC means running setup again.
