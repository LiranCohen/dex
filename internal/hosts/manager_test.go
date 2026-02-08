package hosts

import (
	"strings"
	"testing"
)

func TestManagerDisabled(t *testing.T) {
	m := NewManager()
	m.SetDisabled(true)

	// All operations should succeed silently when disabled
	if err := m.Add("test.example.com"); err != nil {
		t.Errorf("Add should succeed when disabled: %v", err)
	}

	if err := m.Remove("test.example.com"); err != nil {
		t.Errorf("Remove should succeed when disabled: %v", err)
	}

	if err := m.SetHostnames([]string{"a.com", "b.com"}); err != nil {
		t.Errorf("SetHostnames should succeed when disabled: %v", err)
	}

	if err := m.Cleanup(); err != nil {
		t.Errorf("Cleanup should succeed when disabled: %v", err)
	}

	// No hostnames should be tracked when disabled
	if len(m.ManagedHostnames()) != 0 {
		t.Errorf("expected no managed hostnames when disabled, got %v", m.ManagedHostnames())
	}
}

func TestManagerEmptyHostname(t *testing.T) {
	m := NewManager()
	m.SetDisabled(true)

	// Empty hostname should be ignored
	if err := m.Add(""); err != nil {
		t.Errorf("Add empty should succeed: %v", err)
	}

	if err := m.Remove(""); err != nil {
		t.Errorf("Remove empty should succeed: %v", err)
	}
}

func TestParseHostsContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantOurs []string
		wantRest []string
	}{
		{
			name:     "empty file",
			content:  ``,
			wantOurs: nil,
			wantRest: nil,
		},
		{
			name: "standard hosts file",
			content: `127.0.0.1 localhost
::1 localhost
`,
			wantOurs: nil,
			wantRest: []string{"127.0.0.1 localhost", "::1 localhost"},
		},
		{
			name: "with managed block",
			content: `127.0.0.1 localhost
# BEGIN dex-managed entries - do not edit this block
127.0.0.1 hq.test.enbox.id
127.0.0.1 git.test.enbox.id
# END dex-managed entries
`,
			wantOurs: []string{"hq.test.enbox.id", "git.test.enbox.id"},
			wantRest: []string{"127.0.0.1 localhost"},
		},
		{
			name: "managed block in middle",
			content: `127.0.0.1 localhost
# BEGIN dex-managed entries - do not edit this block
127.0.0.1 hq.old.enbox.id
# END dex-managed entries
192.168.1.1 myserver
`,
			wantOurs: []string{"hq.old.enbox.id"},
			wantRest: []string{"127.0.0.1 localhost", "192.168.1.1 myserver"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ours, rest := parseHostsContent(tt.content)

			if len(ours) != len(tt.wantOurs) {
				t.Errorf("ours = %v, want %v", ours, tt.wantOurs)
			}
			for i, h := range ours {
				if i < len(tt.wantOurs) && h != tt.wantOurs[i] {
					t.Errorf("ours[%d] = %q, want %q", i, h, tt.wantOurs[i])
				}
			}

			if len(rest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestBuildHostsContent(t *testing.T) {
	tests := []struct {
		name      string
		existing  string
		hostnames []string
		wantBlock bool
	}{
		{
			name:      "add to empty",
			existing:  "",
			hostnames: []string{"hq.test.enbox.id"},
			wantBlock: true,
		},
		{
			name:      "add to existing",
			existing:  "127.0.0.1 localhost\n",
			hostnames: []string{"hq.test.enbox.id", "git.test.enbox.id"},
			wantBlock: true,
		},
		{
			name:      "remove all",
			existing:  "127.0.0.1 localhost\n# BEGIN dex-managed entries - do not edit this block\n127.0.0.1 old.enbox.id\n# END dex-managed entries\n",
			hostnames: nil,
			wantBlock: false,
		},
		{
			name:      "replace existing",
			existing:  "127.0.0.1 localhost\n# BEGIN dex-managed entries - do not edit this block\n127.0.0.1 old.enbox.id\n# END dex-managed entries\n",
			hostnames: []string{"new.enbox.id"},
			wantBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostnameMap := make(map[string]bool)
			for _, h := range tt.hostnames {
				hostnameMap[h] = true
			}

			result := buildHostsContent(tt.existing, hostnameMap)

			hasBlock := strings.Contains(result, markerStart)
			if hasBlock != tt.wantBlock {
				t.Errorf("hasBlock = %v, want %v\ncontent:\n%s", hasBlock, tt.wantBlock, result)
			}

			// Verify all hostnames are present
			for _, h := range tt.hostnames {
				if !strings.Contains(result, h) {
					t.Errorf("result should contain %q\ncontent:\n%s", h, result)
				}
			}

			// Verify old managed entries are removed
			if strings.Contains(result, "old.enbox.id") && !contains(tt.hostnames, "old.enbox.id") {
				t.Errorf("result should not contain old.enbox.id\ncontent:\n%s", result)
			}

			// Verify non-managed entries are preserved
			if strings.Contains(tt.existing, "localhost") && !strings.Contains(result, "localhost") {
				t.Errorf("result should preserve localhost\ncontent:\n%s", result)
			}
		})
	}
}

// parseHostsContent parses hosts file content and separates our managed entries.
func parseHostsContent(content string) (ours []string, rest []string) {
	lines := strings.Split(content, "\n")
	inManagedBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == markerStart {
			inManagedBlock = true
			continue
		}
		if trimmed == markerEnd {
			inManagedBlock = false
			continue
		}

		if inManagedBlock {
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				fields := strings.Fields(trimmed)
				if len(fields) >= 2 {
					ours = append(ours, fields[1])
				}
			}
			continue
		}

		if trimmed != "" {
			rest = append(rest, line)
		}
	}

	return ours, rest
}

// buildHostsContent builds new hosts file content with managed entries.
func buildHostsContent(existing string, hostnames map[string]bool) string {
	lines := strings.Split(existing, "\n")
	var newLines []string
	inManagedBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == markerStart {
			inManagedBlock = true
			continue
		}
		if trimmed == markerEnd {
			inManagedBlock = false
			continue
		}
		if inManagedBlock {
			continue
		}

		newLines = append(newLines, line)
	}

	// Remove trailing empty lines
	for len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Add managed block if we have hostnames
	if len(hostnames) > 0 {
		newLines = append(newLines, "")
		newLines = append(newLines, markerStart)
		for hostname := range hostnames {
			newLines = append(newLines, loopbackIP+" "+hostname)
		}
		newLines = append(newLines, markerEnd)
	}

	newLines = append(newLines, "")
	return strings.Join(newLines, "\n")
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
