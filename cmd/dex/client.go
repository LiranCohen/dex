package main

import (
	"fmt"
	"os"
)

func printClientUsage() {
	fmt.Fprintf(os.Stderr, "Dex Client - Join the mesh network from your local machine\n\n")
	fmt.Fprintf(os.Stderr, "Usage: dex client <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  enroll    Enroll this device with an HQ enrollment key\n")
	fmt.Fprintf(os.Stderr, "  start     Start the mesh client (connects to mesh network)\n")
	fmt.Fprintf(os.Stderr, "  tray      Start with system tray icon (auto-connects)\n")
	fmt.Fprintf(os.Stderr, "  status    Show connection status\n")
	fmt.Fprintf(os.Stderr, "  stop      Stop the running mesh client\n")
	fmt.Fprintf(os.Stderr, "  help      Show this help message\n")
	fmt.Fprintf(os.Stderr, "\nRun 'dex client <command> --help' for more information on a command.\n")
	fmt.Fprintf(os.Stderr, "\nData directory: %s\n", DefaultClientDataDir())
}

func runClient(args []string) error {
	if len(args) == 0 {
		// Default behavior: start the client
		return runClientStart(args)
	}

	switch args[0] {
	case "enroll":
		return runClientEnroll(args[1:])
	case "start":
		return runClientStart(args[1:])
	case "tray":
		return runClientTray(args[1:])
	case "status":
		return runClientStatus(args[1:])
	case "stop":
		return runClientStop(args[1:])
	case "help", "-h", "--help":
		printClientUsage()
		return nil
	default:
		return fmt.Errorf("unknown client command: %s\nRun 'dex client help' for usage", args[0])
	}
}
