//go:build !darwin

package main

import "fmt"

func runMeshdInstall(_ []string) error {
	return fmt.Errorf("system daemon installation is only supported on macOS currently")
}

func runMeshdUninstall(_ []string) error {
	return fmt.Errorf("system daemon uninstallation is only supported on macOS currently")
}
