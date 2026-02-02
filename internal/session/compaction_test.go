package session

import (
	"strings"
	"testing"

	"github.com/lirancohen/dex/internal/toolbelt"
)

func TestEstimateTokens_EmptyMessages(t *testing.T) {
	tokens := EstimateTokens(nil, "")
	if tokens != 0 {
		t.Errorf("Expected 0 tokens for empty input, got %d", tokens)
	}
}

func TestEstimateTokens_SystemPromptOnly(t *testing.T) {
	systemPrompt := strings.Repeat("x", 400) // 400 chars = ~100 tokens
	tokens := EstimateTokens(nil, systemPrompt)
	if tokens != 100 {
		t.Errorf("Expected ~100 tokens for 400 char system prompt, got %d", tokens)
	}
}

func TestEstimateTokens_StringContent(t *testing.T) {
	messages := []toolbelt.AnthropicMessage{
		{Role: "user", Content: strings.Repeat("x", 400)},
		{Role: "assistant", Content: strings.Repeat("y", 200)},
	}
	tokens := EstimateTokens(messages, "")
	expected := 150 // 400/4 + 200/4
	if tokens != expected {
		t.Errorf("Expected %d tokens, got %d", expected, tokens)
	}
}

func TestFilterToolResponses_NoRemoval(t *testing.T) {
	messages := []toolbelt.AnthropicMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	result := filterToolResponses(messages, 0)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestFilterToolResponses_RemoveAll(t *testing.T) {
	messages := []toolbelt.AnthropicMessage{
		{Role: "user", Content: "start"},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result1"}}},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result2"}}},
		{Role: "assistant", Content: "done"},
	}
	result := filterToolResponses(messages, 100)
	// Should remove both tool responses
	if len(result) != 2 {
		t.Errorf("Expected 2 messages after removing all tool responses, got %d", len(result))
	}
}

func TestFilterToolResponses_Partial(t *testing.T) {
	messages := []toolbelt.AnthropicMessage{
		{Role: "user", Content: "start"},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result1"}}},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result2"}}},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result3"}}},
		{Role: "user", Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result4"}}},
		{Role: "assistant", Content: "done"},
	}
	result := filterToolResponses(messages, 50) // Remove 50% = 2 out of 4
	if len(result) != 4 { // 6 - 2 = 4
		t.Errorf("Expected 4 messages after removing 50%% tool responses, got %d", len(result))
	}
}

func TestHasToolResponse_True(t *testing.T) {
	msg := toolbelt.AnthropicMessage{
		Role:    "user",
		Content: []toolbelt.ContentBlock{{Type: "tool_result", Content: "result"}},
	}
	if !hasToolResponse(msg) {
		t.Error("Expected hasToolResponse to return true for tool_result")
	}
}

func TestHasToolResponse_False(t *testing.T) {
	msg := toolbelt.AnthropicMessage{
		Role:    "user",
		Content: "just text",
	}
	if hasToolResponse(msg) {
		t.Error("Expected hasToolResponse to return false for text content")
	}
}

func TestContextGuard_NoCompactionNeeded(t *testing.T) {
	guard := NewContextGuard(nil)

	// Small message list - shouldn't need compaction
	messages := []toolbelt.AnthropicMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	result, compacted, err := guard.CheckAndCompact(messages, "system", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if compacted {
		t.Error("Expected no compaction for small messages")
	}
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestExtractFirstSentence(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello world.", "Hello world."},
		{"First sentence. Second sentence.", "First sentence."},
		{"Question? Answer.", "Question?"},
		{"Exclaim! More.", "Exclaim!"},
		{"No ending", "No ending"},
		{strings.Repeat("x", 200), strings.Repeat("x", 100) + "..."},
	}

	for _, tc := range tests {
		result := extractFirstSentence(tc.input)
		if result != tc.expected {
			t.Errorf("extractFirstSentence(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestExtractQualityResult(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"QUALITY_PASSED: all tests pass", "passed"},
		{"QUALITY_BLOCKED: tests failed", "blocked"},
		{"other content", ""},
	}

	for _, tc := range tests {
		result := extractQualityResult(tc.input)
		if result != tc.expected {
			t.Errorf("extractQualityResult(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSummarizeMessages(t *testing.T) {
	messages := []toolbelt.AnthropicMessage{
		{Role: "assistant", Content: "I decided to use this approach"},
		{Role: "user", Content: "QUALITY_PASSED: all tests pass"},
	}

	summary := summarizeMessages(messages)

	if !strings.Contains(summary, "Decision") {
		t.Error("Expected summary to contain decision")
	}
	if !strings.Contains(summary, "Quality gate: passed") {
		t.Error("Expected summary to contain quality gate result")
	}
}
