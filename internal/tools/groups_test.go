package tools

import (
	"testing"
)

func TestGetToolByName(t *testing.T) {
	tool := GetToolByName("read_file")
	if tool == nil {
		t.Fatal("Expected to find read_file tool")
	}
	if tool.Name != "read_file" {
		t.Errorf("Expected name 'read_file', got '%s'", tool.Name)
	}
}

func TestGetToolByName_NotFound(t *testing.T) {
	tool := GetToolByName("nonexistent_tool")
	if tool != nil {
		t.Error("Expected nil for nonexistent tool")
	}
}

func TestGetToolsForHat_Explorer(t *testing.T) {
	toolSet := GetToolsForHat("explorer")

	// Explorer should have read-only tools
	if !toolSet.Has("read_file") {
		t.Error("Explorer should have read_file")
	}
	if !toolSet.Has("grep") {
		t.Error("Explorer should have grep")
	}
	if !toolSet.Has("git_status") {
		t.Error("Explorer should have git_status")
	}
	if !toolSet.Has("web_search") {
		t.Error("Explorer should have web_search")
	}

	// Explorer should NOT have write tools
	if toolSet.Has("write_file") {
		t.Error("Explorer should NOT have write_file")
	}
	if toolSet.Has("bash") {
		t.Error("Explorer should NOT have bash")
	}
	if toolSet.Has("git_commit") {
		t.Error("Explorer should NOT have git_commit")
	}
}

func TestGetToolsForHat_Creator(t *testing.T) {
	toolSet := GetToolsForHat("creator")

	// Creator should have both read and write tools
	if !toolSet.Has("read_file") {
		t.Error("Creator should have read_file")
	}
	if !toolSet.Has("write_file") {
		t.Error("Creator should have write_file")
	}
	if !toolSet.Has("bash") {
		t.Error("Creator should have bash")
	}
	if !toolSet.Has("git_commit") {
		t.Error("Creator should have git_commit")
	}
	if !toolSet.Has("run_tests") {
		t.Error("Creator should have run_tests")
	}

	// Creator should NOT have completion tools (that's editor)
	if toolSet.Has("task_complete") {
		t.Error("Creator should NOT have task_complete")
	}
}

func TestGetToolsForHat_Editor(t *testing.T) {
	toolSet := GetToolsForHat("editor")

	// Editor should have everything including completion
	if !toolSet.Has("read_file") {
		t.Error("Editor should have read_file")
	}
	if !toolSet.Has("write_file") {
		t.Error("Editor should have write_file")
	}
	if !toolSet.Has("task_complete") {
		t.Error("Editor should have task_complete")
	}
	if !toolSet.Has("github_create_pr") {
		t.Error("Editor should have github_create_pr")
	}
}

func TestGetToolsForHat_Critic(t *testing.T) {
	toolSet := GetToolsForHat("critic")

	// Critic should have read-only tools plus quality tools
	if !toolSet.Has("read_file") {
		t.Error("Critic should have read_file")
	}
	if !toolSet.Has("git_diff") {
		t.Error("Critic should have git_diff")
	}

	// Critic should NOT have write tools
	if toolSet.Has("write_file") {
		t.Error("Critic should NOT have write_file")
	}
	if toolSet.Has("git_commit") {
		t.Error("Critic should NOT have git_commit")
	}
	if toolSet.Has("bash") {
		t.Error("Critic should NOT have bash")
	}
}

func TestGetToolsForHat_UnknownHat(t *testing.T) {
	toolSet := GetToolsForHat("unknown_hat")

	// Should default to explorer profile (safe)
	if !toolSet.Has("read_file") {
		t.Error("Unknown hat should default to explorer with read_file")
	}
	if toolSet.Has("write_file") {
		t.Error("Unknown hat should NOT have write_file")
	}
}

func TestIsToolAllowedForHat(t *testing.T) {
	tests := []struct {
		tool    string
		hat     string
		allowed bool
	}{
		{"read_file", "explorer", true},
		{"read_file", "creator", true},
		{"write_file", "explorer", false},
		{"write_file", "creator", true},
		{"bash", "explorer", false},
		{"bash", "creator", true},
		{"task_complete", "creator", false},
		{"task_complete", "editor", true},
		{"git_commit", "critic", false},
		{"git_commit", "creator", true},
	}

	for _, tc := range tests {
		result := IsToolAllowedForHat(tc.tool, tc.hat)
		if result != tc.allowed {
			t.Errorf("IsToolAllowedForHat(%q, %q) = %v, want %v", tc.tool, tc.hat, result, tc.allowed)
		}
	}
}

func TestGetProfileForHat(t *testing.T) {
	tests := []struct {
		hat     string
		profile ToolProfile
	}{
		{"explorer", ProfileExplorer},
		{"planner", ProfilePlanner},
		{"designer", ProfilePlanner},
		{"creator", ProfileCreator},
		{"critic", ProfileCritic},
		{"editor", ProfileEditor},
		{"resolver", ProfileCreator},
	}

	for _, tc := range tests {
		result := GetProfileForHat(tc.hat)
		if result != tc.profile {
			t.Errorf("GetProfileForHat(%q) = %v, want %v", tc.hat, result, tc.profile)
		}
	}
}

func TestGetToolsInGroup(t *testing.T) {
	fsReadTools := GetToolsInGroup(GroupFSRead)
	if len(fsReadTools) != 4 {
		t.Errorf("Expected 4 tools in GroupFSRead, got %d", len(fsReadTools))
	}

	// Verify expected tools are present
	expected := map[string]bool{
		"read_file":  true,
		"list_files": true,
		"glob":       true,
		"grep":       true,
	}
	for _, tool := range fsReadTools {
		if !expected[tool] {
			t.Errorf("Unexpected tool in GroupFSRead: %s", tool)
		}
	}
}

func TestListRegisteredTools(t *testing.T) {
	tools := ListRegisteredTools()
	if len(tools) < 20 {
		t.Errorf("Expected at least 20 registered tools, got %d", len(tools))
	}
}

func TestAllToolGroupsCoverAllTools(t *testing.T) {
	// Get all registered tools
	registeredTools := make(map[string]bool)
	for _, name := range ListRegisteredTools() {
		registeredTools[name] = true
	}

	// Get all tools in all groups
	groupedTools := make(map[string]bool)
	for _, group := range GetAllToolGroups() {
		for _, name := range GetToolsInGroup(group) {
			groupedTools[name] = true
		}
	}

	// Every registered tool should be in at least one group
	for tool := range registeredTools {
		if !groupedTools[tool] {
			t.Errorf("Registered tool %q is not in any group", tool)
		}
	}
}
