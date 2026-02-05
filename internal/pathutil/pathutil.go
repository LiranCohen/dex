// Package pathutil provides shared path validation utilities.
package pathutil

import (
	"path/filepath"
	"strings"
)

// systemPaths are directories that should never be used for project code.
var systemPaths = []string{
	"/usr",
	"/bin",
	"/sbin",
	"/lib",
	"/lib64",
	"/etc",
	"/var",
	"/root",
	"/boot",
	"/dev",
	"/proc",
	"/sys",
}

// IsValidProjectPath checks if the given path is appropriate for use as a
// project directory. Returns false for empty paths, system directories, and
// optionally for the dex base directory itself (pass "" to skip that check).
func IsValidProjectPath(path, baseDir string) bool {
	if path == "" || path == "." || path == ".." {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, sysPath := range systemPaths {
		if absPath == sysPath || strings.HasPrefix(absPath, sysPath+"/") {
			return false
		}
	}

	// Reject the dex base directory itself (but allow subdirectories like repos/)
	if baseDir != "" {
		dexBase, err := filepath.Abs(baseDir)
		if err == nil && absPath == dexBase {
			return false
		}
	}

	return true
}
