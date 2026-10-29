//go:build !windows

package main

import (
	"errors"
	"os"
)

// Outside Windows the calendar URL comes from an environment variable and is
// never stored.
const feedURLEnv = "MEETING_REMINDER_ICAL_URL"

func acquireSingleInstance() bool { return true }

func loadFeedURL() (string, error) {
	if u := os.Getenv(feedURLEnv); u != "" {
		return u, nil
	}
	return "", errors.New(feedURLEnv + " is not set")
}

func saveFeedURL(string) error { return nil }

func promptFeedURL() (string, error) { return loadFeedURL() }
