// Package session provides session lifecycle management for Poindexter
package session

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Event represents a pub/sub event for hat coordination
type Event struct {
	ID        string
	SessionID string
	Topic     string
	Payload   string // JSON payload (optional)
	SourceHat string
	CreatedAt time.Time
}

// Event signal prefix
const SignalEvent = "EVENT:"

// Event topics - these define the pub/sub contract between hats
const (
	// Task lifecycle events
	TopicTaskStarted  = "task.started"  // System publishes when task starts
	TopicTaskComplete = "task.complete" // Terminal: task finished successfully
	TopicTaskBlocked  = "task.blocked"  // Any hat can publish when blocked

	// Planning/design events
	TopicPlanComplete   = "plan.complete"   // Planner publishes when plan ready
	TopicDesignComplete = "design.complete" // Designer/Planner publishes when design ready

	// Implementation events
	TopicImplementationDone = "implementation.done" // Creator publishes when implementation complete

	// Review events
	TopicReviewApproved = "review.approved" // Critic publishes when review passes
	TopicReviewRejected = "review.rejected" // Critic publishes when review fails

	// Resolution events
	TopicResolved = "resolved" // Resolver publishes when blocker resolved
)

// IsTerminalEvent returns true if the topic indicates task completion
func IsTerminalEvent(topic string) bool {
	return topic == TopicTaskComplete
}

// ParseEvent extracts an EVENT:topic or EVENT:topic:{"json"} from text
// Returns the Event and true if found, nil and false otherwise
func ParseEvent(text, sessionID, sourceHat string) (*Event, bool) {
	idx := strings.Index(text, SignalEvent)
	if idx == -1 {
		return nil, false
	}

	// Extract content after EVENT:
	remaining := text[idx+len(SignalEvent):]

	// Find end of topic (whitespace, newline, or end of string)
	// Also handle EVENT:topic:{"payload"} format
	var topic, payload string

	// First, check if there's a JSON payload after the topic
	colonCount := 0
	topicEnd := -1
	jsonStart := -1

	for i, c := range remaining {
		if c == ':' {
			colonCount++
			if colonCount == 1 {
				// This might be the separator between topic and JSON payload
				// Look ahead for JSON start
				rest := remaining[i+1:]
				if len(rest) > 0 && (rest[0] == '{' || rest[0] == '"') {
					topicEnd = i
					jsonStart = i + 1
					break
				}
			}
		} else if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			topicEnd = i
			break
		}
	}

	if topicEnd == -1 {
		topicEnd = len(remaining)
	}

	topic = strings.TrimSpace(remaining[:topicEnd])

	// Extract JSON payload if present
	if jsonStart != -1 {
		jsonContent := remaining[jsonStart:]
		// Find the end of JSON (matching braces or end of line)
		payload = extractJSON(jsonContent)
	}

	// Validate topic
	if topic == "" || !isValidTopic(topic) {
		return nil, false
	}

	return &Event{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Topic:     topic,
		Payload:   payload,
		SourceHat: sourceHat,
		CreatedAt: time.Now(),
	}, true
}

// extractJSON extracts a JSON object or value from the start of a string
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}

	// Handle JSON object
	if s[0] == '{' {
		depth := 0
		inString := false
		escaped := false

		for i, c := range s {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString {
				escaped = true
				continue
			}
			if c == '"' {
				inString = !inString
				continue
			}
			if !inString {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						return s[:i+1]
					}
				}
			}
		}
	}

	// Handle simple quoted string
	if s[0] == '"' {
		escaped := false
		for i := 1; i < len(s); i++ {
			if escaped {
				escaped = false
				continue
			}
			if s[i] == '\\' {
				escaped = true
				continue
			}
			if s[i] == '"' {
				return s[:i+1]
			}
		}
	}

	// Take until whitespace or newline
	endIdx := strings.IndexAny(s, " \t\n\r")
	if endIdx == -1 {
		return s
	}
	return s[:endIdx]
}

// isValidTopic checks if the topic is a known event topic
func isValidTopic(topic string) bool {
	validTopics := []string{
		TopicTaskStarted,
		TopicTaskComplete,
		TopicTaskBlocked,
		TopicPlanComplete,
		TopicDesignComplete,
		TopicImplementationDone,
		TopicReviewApproved,
		TopicReviewRejected,
		TopicResolved,
	}

	for _, t := range validTopics {
		if topic == t {
			return true
		}
	}
	return false
}

// GetPayloadValue extracts a value from the JSON payload
func (e *Event) GetPayloadValue(key string) (string, bool) {
	if e.Payload == "" {
		return "", false
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(e.Payload), &data); err != nil {
		return "", false
	}

	val, ok := data[key]
	if !ok {
		return "", false
	}

	switch v := val.(type) {
	case string:
		return v, true
	default:
		// Convert to JSON string for non-string values
		b, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}
