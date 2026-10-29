//go:build !windows

package main

import "fmt"

func showInfo(msg string) { fmt.Println(msg) }

func showError(msg string) { fmt.Println("error:", msg) }
