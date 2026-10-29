# 0001. Read the calendar from the secret iCal feed

- Status: Accepted
- Date: 2026-10-29

## Context

The app needs read access to one person's Google Calendar on a work PC. The
options were:

- **Google Calendar API with OAuth.** Needs a Google Cloud project, an OAuth
  client and a consent screen. While the app is in "Testing", refresh tokens
  expire every 7 days. A public repo can't ship the client credentials, so
  every user would need their own project.
- **Apps Script relay.** No Cloud project, but Workspace domains often
  restrict web-app access to signed-in users, which a background exe can't do.
- **The calendar's "Secret address in iCal format".** A plain HTTPS URL that
  returns the whole calendar as an `.ics` file, rebuilt on every request.

## Decision

Read the secret iCal address. The app downloads and parses the feed itself,
including recurrence rules, exceptions and time zones.

## Consequences

- Setup is a single copy-paste, with nothing in Google Cloud and nothing
  secret in the repo.
- The URL is a bearer secret: anyone who has it can read the calendar. It must
  be stored securely ([0005](0005-store-feed-url-in-credential-manager.md))
  and kept out of logs, and it can be reset in Google Calendar.
- A Workspace admin can disable the feature. OAuth would be the fallback.
- The app owns recurrence expansion, which the API would otherwise do. This is
  the riskiest logic in the app and is covered by tests.
- The feed has no change detection, so every poll is a full download. Polling
  every 5 minutes with gzip keeps this small.
