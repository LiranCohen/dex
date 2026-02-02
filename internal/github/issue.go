package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v68/github"
)

// RetryConfig configures retry behavior for GitHub API calls
type RetryConfig struct {
	MaxAttempts int           // Maximum number of attempts (default: 3)
	InitialWait time.Duration // Initial wait time (default: 1s)
	MaxWait     time.Duration // Maximum wait time (default: 30s)
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 1 * time.Second,
		MaxWait:     30 * time.Second,
	}
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error, resp *github.Response) bool {
	if err == nil {
		return false
	}

	// Check for rate limiting
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		return true
	}

	// Check for server errors (5xx)
	if resp != nil && resp.StatusCode >= 500 {
		return true
	}

	// Check for specific error types
	if _, ok := err.(*github.RateLimitError); ok {
		return true
	}
	if _, ok := err.(*github.AbuseRateLimitError); ok {
		return true
	}

	return false
}

// retryWithBackoff executes a function with retry and exponential backoff
func retryWithBackoff[T any](ctx context.Context, cfg RetryConfig, operation func() (T, *github.Response, error)) (T, error) {
	var lastErr error
	var zero T
	wait := cfg.InitialWait

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		result, resp, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if !isRetryableError(err, resp) {
			return zero, err
		}

		// Don't retry on last attempt
		if attempt >= cfg.MaxAttempts {
			break
		}

		// Check for rate limit reset time
		if resp != nil && resp.Rate.Remaining == 0 {
			resetTime := resp.Rate.Reset.Time
			waitUntilReset := time.Until(resetTime)
			if waitUntilReset > 0 && waitUntilReset < cfg.MaxWait {
				wait = waitUntilReset + time.Second // Add 1s buffer
			}
		}

		// Cap wait time
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}

		fmt.Printf("GitHub API error (attempt %d/%d), retrying in %v: %v\n", attempt, cfg.MaxAttempts, wait, err)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}

		// Exponential backoff
		wait *= 2
	}

	return zero, fmt.Errorf("after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// Issue labels used by Dex
const (
	LabelQuest     = "dex:quest"
	LabelObjective = "dex:objective"
)

// IssueOptions configures issue creation
type IssueOptions struct {
	Title  string
	Body   string
	Labels []string
}

// CreateQuestIssue creates a GitHub Issue for a Quest
func CreateQuestIssue(ctx context.Context, client *github.Client, owner, repo string, opts IssueOptions) (*github.Issue, error) {
	// Ensure quest label is present
	labels := append(opts.Labels, LabelQuest)

	req := &github.IssueRequest{
		Title:  github.Ptr(opts.Title),
		Body:   github.Ptr(opts.Body),
		Labels: &labels,
	}

	issue, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.Issue, *github.Response, error) {
		return client.Issues.Create(ctx, owner, repo, req)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create quest issue: %w", err)
	}

	return issue, nil
}

// CreateObjectiveIssue creates a GitHub Issue for a Task/Objective
func CreateObjectiveIssue(ctx context.Context, client *github.Client, owner, repo string, opts IssueOptions, questIssueNumber int) (*github.Issue, error) {
	// Ensure objective label is present
	labels := append(opts.Labels, LabelObjective)

	// Add reference to parent quest in body if provided
	body := opts.Body
	if questIssueNumber > 0 {
		body = fmt.Sprintf("%s\n\n---\nPart of Quest #%d", opts.Body, questIssueNumber)
	}

	req := &github.IssueRequest{
		Title:  github.Ptr(opts.Title),
		Body:   github.Ptr(body),
		Labels: &labels,
	}

	issue, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.Issue, *github.Response, error) {
		return client.Issues.Create(ctx, owner, repo, req)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create objective issue: %w", err)
	}

	return issue, nil
}

// UpdateIssueBody updates the body of an existing issue
func UpdateIssueBody(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, body string) error {
	req := &github.IssueRequest{
		Body: github.Ptr(body),
	}

	_, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.Issue, *github.Response, error) {
		return client.Issues.Edit(ctx, owner, repo, issueNumber, req)
	})
	if err != nil {
		return fmt.Errorf("failed to update issue body: %w", err)
	}

	return nil
}

// AddIssueComment adds a comment to an issue
func AddIssueComment(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, body string) error {
	comment := &github.IssueComment{
		Body: github.Ptr(body),
	}

	_, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.IssueComment, *github.Response, error) {
		return client.Issues.CreateComment(ctx, owner, repo, issueNumber, comment)
	})
	if err != nil {
		return fmt.Errorf("failed to add issue comment: %w", err)
	}

	return nil
}

// CloseIssue closes an issue
func CloseIssue(ctx context.Context, client *github.Client, owner, repo string, issueNumber int) error {
	req := &github.IssueRequest{
		State: github.Ptr("closed"),
	}

	_, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.Issue, *github.Response, error) {
		return client.Issues.Edit(ctx, owner, repo, issueNumber, req)
	})
	if err != nil {
		return fmt.Errorf("failed to close issue: %w", err)
	}

	return nil
}

// CloseIssueWithComment closes an issue with a final comment
func CloseIssueWithComment(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, comment string) error {
	// Add comment first
	if comment != "" {
		if err := AddIssueComment(ctx, client, owner, repo, issueNumber, comment); err != nil {
			return err
		}
	}

	// Then close
	return CloseIssue(ctx, client, owner, repo, issueNumber)
}

// ReopenIssue reopens a closed issue
func ReopenIssue(ctx context.Context, client *github.Client, owner, repo string, issueNumber int) error {
	req := &github.IssueRequest{
		State: github.Ptr("open"),
	}

	_, err := retryWithBackoff(ctx, DefaultRetryConfig(), func() (*github.Issue, *github.Response, error) {
		return client.Issues.Edit(ctx, owner, repo, issueNumber, req)
	})
	if err != nil {
		return fmt.Errorf("failed to reopen issue: %w", err)
	}

	return nil
}

// ReopenIssueWithComment reopens an issue with a comment
func ReopenIssueWithComment(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, comment string) error {
	// Reopen first
	if err := ReopenIssue(ctx, client, owner, repo, issueNumber); err != nil {
		return err
	}

	// Then add comment
	if comment != "" {
		if err := AddIssueComment(ctx, client, owner, repo, issueNumber, comment); err != nil {
			return err
		}
	}

	return nil
}

// LinkPRToIssue adds a comment to an issue linking to a PR
func LinkPRToIssue(ctx context.Context, client *github.Client, owner, repo string, issueNumber, prNumber int) error {
	body := fmt.Sprintf("PR #%d opened for this objective.", prNumber)
	return AddIssueComment(ctx, client, owner, repo, issueNumber, body)
}

// LinkObjectiveToQuest adds a comment to a quest issue when an objective is created
func LinkObjectiveToQuest(ctx context.Context, client *github.Client, owner, repo string, questIssueNumber, objectiveIssueNumber int, objectiveTitle string) error {
	body := fmt.Sprintf("Objective created: #%d - %s", objectiveIssueNumber, objectiveTitle)
	return AddIssueComment(ctx, client, owner, repo, questIssueNumber, body)
}

// EnsureLabelsExist creates the dex labels if they don't exist
func EnsureLabelsExist(ctx context.Context, client *github.Client, owner, repo string) error {
	labels := []struct {
		name        string
		color       string
		description string
	}{
		{LabelQuest, "7057ff", "Dex Quest - a conversation that spawns objectives"},
		{LabelObjective, "0e8a16", "Dex Objective - a task being worked on by an AI agent"},
	}

	for _, label := range labels {
		_, resp, err := client.Issues.GetLabel(ctx, owner, repo, label.name)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				// Label doesn't exist, create it
				_, _, err = client.Issues.CreateLabel(ctx, owner, repo, &github.Label{
					Name:        github.Ptr(label.name),
					Color:       github.Ptr(label.color),
					Description: github.Ptr(label.description),
				})
				if err != nil {
					return fmt.Errorf("failed to create label %s: %w", label.name, err)
				}
			} else {
				return fmt.Errorf("failed to check label %s: %w", label.name, err)
			}
		}
	}

	return nil
}

// FormatQuestIssueBody formats the body for a quest issue
func FormatQuestIssueBody(description string, projectName string) string {
	return fmt.Sprintf(`## Quest

%s

---

**Project:** %s

### Objectives

_Objectives will be linked here as they are created._
`, description, projectName)
}

// FormatObjectiveIssueBody formats the body for an objective issue
func FormatObjectiveIssueBody(description string, checklist []string) string {
	body := fmt.Sprintf("## Objective\n\n%s\n", description)

	if len(checklist) > 0 {
		body += "\n### Checklist\n\n"
		for _, item := range checklist {
			body += fmt.Sprintf("- [ ] %s\n", item)
		}
	}

	return body
}

// ChecklistItemWithStatus represents a checklist item with its completion status
type ChecklistItemWithStatus struct {
	Description string
	Status      string // pending, in_progress, done, failed, skipped
}

// FormatObjectiveIssueBodyWithStatus formats the body with checklist item statuses
func FormatObjectiveIssueBodyWithStatus(description string, checklist []ChecklistItemWithStatus) string {
	body := fmt.Sprintf("## Objective\n\n%s\n", description)

	if len(checklist) > 0 {
		body += "\n### Checklist\n\n"
		for _, item := range checklist {
			checkbox := "[ ]"
			suffix := ""
			switch item.Status {
			case "done":
				checkbox = "[x]"
			case "in_progress":
				suffix = " *(in progress)*"
			case "failed":
				checkbox = "[x]"
				suffix = " *(failed)*"
			case "skipped":
				checkbox = "[x]"
				suffix = " *(skipped)*"
			}
			body += fmt.Sprintf("- %s %s%s\n", checkbox, item.Description, suffix)
		}
	}

	return body
}

// IssueURL returns the URL for an issue given owner, repo, and number
func IssueURL(owner, repo string, number int) string {
	return fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, repo, number)
}
