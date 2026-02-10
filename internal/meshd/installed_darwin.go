package meshd

import "os"

const launchDaemonPlist = "/Library/LaunchDaemons/com.dex.meshd.plist"

// isInstalled checks if the daemon is installed as a macOS LaunchDaemon.
func isInstalled() bool {
	_, err := os.Stat(launchDaemonPlist)
	return err == nil
}
