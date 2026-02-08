//go:build notray

package main

import "fmt"

// runClientTray is a stub when tray support is disabled
func runClientTray(args []string) error {
	return fmt.Errorf("tray support not available in this build. Use 'dex client start' instead")
}
