//go:build !windows

package main

import (
	"fmt"
	"time"
)

// On non-Windows systems alerts go to stdout, so the app can be tried out in
// WSL or Linux during development.

func alert(e Event) { fmt.Printf("\a[ALERT] %s\n", formatAlert(e, time.Now())) }

func showInfo(msg string) { fmt.Println(msg) }

func showError(msg string) { fmt.Println("error:", msg) }
