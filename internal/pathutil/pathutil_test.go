package pathutil

import "testing"

func TestIsValidProjectPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		baseDir string
		want    bool
	}{
		// Invalid empty/relative paths
		{"empty string", "", "", false},
		{"dot", ".", "", false},
		{"dotdot", "..", "", false},

		// System directories — exact match
		{"system /usr", "/usr", "", false},
		{"system /bin", "/bin", "", false},
		{"system /etc", "/etc", "", false},
		{"system /var", "/var", "", false},
		{"system /root", "/root", "", false},
		{"system /dev", "/dev", "", false},
		{"system /proc", "/proc", "", false},
		{"system /sys", "/sys", "", false},

		// System directories — subdirectories
		{"subdir /usr/local", "/usr/local", "", false},
		{"subdir /etc/nginx", "/etc/nginx", "", false},
		{"subdir /var/log", "/var/log", "", false},

		// Valid paths
		{"home directory", "/home/user/project", "", true},
		{"opt subdir", "/opt/dex/repos/myrepo", "", true},
		{"opt itself", "/opt", "", true},
		{"tmp", "/tmp/workdir", "", true},

		// Base directory rejection
		{"base dir exact", "/opt/dex", "/opt/dex", false},
		{"base dir subdir allowed", "/opt/dex/repos", "/opt/dex", true},
		{"base dir empty skip", "/opt/dex", "", true},

		// Paths that look like system paths but aren't
		{"usr-like prefix", "/usr2/project", "", true},
		{"var-like prefix", "/variable/data", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidProjectPath(tt.path, tt.baseDir)
			if got != tt.want {
				t.Errorf("IsValidProjectPath(%q, %q) = %v, want %v", tt.path, tt.baseDir, got, tt.want)
			}
		})
	}
}
