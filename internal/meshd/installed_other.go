//go:build !darwin

package meshd

// isInstalled returns false on non-macOS platforms where daemon installation
// is not yet supported.
func isInstalled() bool {
	return false
}
