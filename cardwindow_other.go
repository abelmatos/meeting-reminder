//go:build !windows

package main

import "fmt"

func runCard() int {
	fmt.Println("-card is only supported on Windows")
	return 2
}
