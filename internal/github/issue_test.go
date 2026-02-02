package github

import (
	"strings"
	"testing"
)

func TestFormatQuestIssueBody(t *testing.T) {
	body := FormatQuestIssueBody("Add user authentication to the app", "my-project")

	if !strings.Contains(body, "## Quest") {
		t.Error("FormatQuestIssueBody should contain Quest header")
	}
	if !strings.Contains(body, "Add user authentication to the app") {
		t.Error("FormatQuestIssueBody should contain description")
	}
	if !strings.Contains(body, "**Project:** my-project") {
		t.Error("FormatQuestIssueBody should contain project name")
	}
	if !strings.Contains(body, "### Objectives") {
		t.Error("FormatQuestIssueBody should contain Objectives section")
	}
}

func TestFormatObjectiveIssueBody(t *testing.T) {
	checklist := []string{"Implement login form", "Add validation", "Connect to auth API"}
	body := FormatObjectiveIssueBody("Create the login page with OAuth support", checklist)

	if !strings.Contains(body, "## Objective") {
		t.Error("FormatObjectiveIssueBody should contain Objective header")
	}
	if !strings.Contains(body, "Create the login page with OAuth support") {
		t.Error("FormatObjectiveIssueBody should contain description")
	}
	if !strings.Contains(body, "### Checklist") {
		t.Error("FormatObjectiveIssueBody should contain Checklist section")
	}
	if !strings.Contains(body, "- [ ] Implement login form") {
		t.Error("FormatObjectiveIssueBody should contain checklist items")
	}
	if !strings.Contains(body, "- [ ] Add validation") {
		t.Error("FormatObjectiveIssueBody should contain all checklist items")
	}
}

func TestFormatObjectiveIssueBodyNoChecklist(t *testing.T) {
	body := FormatObjectiveIssueBody("Simple task without checklist", nil)

	if !strings.Contains(body, "## Objective") {
		t.Error("FormatObjectiveIssueBody should contain Objective header")
	}
	if !strings.Contains(body, "Simple task without checklist") {
		t.Error("FormatObjectiveIssueBody should contain description")
	}
	if strings.Contains(body, "### Checklist") {
		t.Error("FormatObjectiveIssueBody should not contain Checklist section when empty")
	}
}

func TestIssueURL(t *testing.T) {
	tests := []struct {
		owner  string
		repo   string
		number int
		want   string
	}{
		{"lirancohen", "dex", 42, "https://github.com/lirancohen/dex/issues/42"},
		{"myorg", "my-repo", 1, "https://github.com/myorg/my-repo/issues/1"},
	}

	for _, tt := range tests {
		got := IssueURL(tt.owner, tt.repo, tt.number)
		if got != tt.want {
			t.Errorf("IssueURL(%q, %q, %d) = %q, want %q", tt.owner, tt.repo, tt.number, got, tt.want)
		}
	}
}

func TestLabels(t *testing.T) {
	// Verify label constants are set
	if LabelQuest != "dex:quest" {
		t.Errorf("LabelQuest = %q, want %q", LabelQuest, "dex:quest")
	}
	if LabelObjective != "dex:objective" {
		t.Errorf("LabelObjective = %q, want %q", LabelObjective, "dex:objective")
	}
}
