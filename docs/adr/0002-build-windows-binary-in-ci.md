# 0002. Build the Windows binary only in CI, in pure Go

- Status: Accepted
- Date: 2026-10-29

## Context

The app runs on Windows, but development happens in WSL, and installing a
Windows toolchain (or a C compiler for cgo) on the work PC is undesirable.

## Decision

- Write the app in Go without cgo, so it cross-compiles from any OS. Windows
  APIs are called directly through system DLLs.
- Build, vet and test the Windows binary in GitHub Actions on a Windows
  runner. Lint runs for both Linux and Windows.
- Ship a single self-contained `reminder.exe`: the time zone database, icon,
  manifest and card page are embedded. Tagged versions are published as
  GitHub Releases with a checksum.
- Keep platform-specific code behind build tags, with stubs, so the logic can
  be developed and tested on Linux.

## Consequences

- Only Go and Git are needed locally. Dependencies must be pure Go on
  Windows, which ruled out cgo-based GUI toolkits.
- Windows-only behaviour (the card, tray, credentials) is compiled and linted
  but not run in CI. It needs a manual check on Windows.
- The exe is unsigned, so SmartScreen warns on first download.
