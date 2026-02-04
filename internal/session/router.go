// Package session provides session lifecycle management for Poindexter
package session

import (
	"encoding/json"
	"fmt"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/realtime"
)

// EventRouter routes events to appropriate hats based on contracts
type EventRouter struct {
	db          *db.DB
	tracker     *TransitionTracker
	broadcaster *realtime.Broadcaster
}

// RouteResult contains the result of routing an event
type RouteResult struct {
	NextHat    string // The hat to transition to (empty if terminal)
	IsTerminal bool   // True if this is a terminal event (task complete)
	Error      error  // Routing error (e.g., loop detected)
}

// NewEventRouter creates a new event router
func NewEventRouter(database *db.DB, tracker *TransitionTracker, broadcaster *realtime.Broadcaster) *EventRouter {
	return &EventRouter{
		db:          database,
		tracker:     tracker,
		broadcaster: broadcaster,
	}
}

// Route determines the next hat based on an event
// Returns the next hat, whether this is a terminal event, and any error
func (r *EventRouter) Route(event *Event, currentHat string) *RouteResult {
	// Check if hat can publish this topic
	if !CanPublish(currentHat, event.Topic) {
		return &RouteResult{
			Error: fmt.Errorf("hat %s cannot publish topic %s", currentHat, event.Topic),
		}
	}

	// Check for terminal events
	if IsTerminalEvent(event.Topic) {
		return &RouteResult{
			IsTerminal: true,
		}
	}

	// Get the next hat based on topic
	nextHat := GetNextHatForTopic(event.Topic)
	if nextHat == "" {
		return &RouteResult{
			Error: fmt.Errorf("no subscriber found for topic %s", event.Topic),
		}
	}

	// Check for transition loops if tracker is available
	if r.tracker != nil {
		if err := r.tracker.RecordTransition(currentHat, nextHat); err != nil {
			return &RouteResult{
				Error: fmt.Errorf("loop detected: %w", err),
			}
		}
	}

	return &RouteResult{
		NextHat: nextHat,
	}
}

// Persist saves an event to the database
func (r *EventRouter) Persist(event *Event) error {
	if r.db == nil {
		return nil // No persistence configured
	}

	_, err := r.db.CreateEvent(event.SessionID, event.Topic, event.Payload, event.SourceHat)
	if err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}
	return nil
}

// RouteAndPersist routes an event and persists it in one operation
// taskID and projectID are provided for Centrifuge channel routing
func (r *EventRouter) RouteAndPersist(event *Event, currentHat, taskID, projectID string) *RouteResult {
	// Persist first (even if routing fails, we want the event recorded)
	if err := r.Persist(event); err != nil {
		// Log but don't fail - persistence is secondary to routing
		fmt.Printf("EventRouter: warning - failed to persist event: %v\n", err)
	}

	// Broadcast hat event to Centrifuge for real-time updates
	if r.broadcaster != nil {
		hatEventType := topicToHatEvent(event.Topic)
		if hatEventType != "" {
			payload := map[string]any{
				"topic":      event.Topic,
				"source_hat": event.SourceHat,
			}
			// Parse and merge event payload if present
			if event.Payload != "" {
				var payloadData map[string]any
				if err := json.Unmarshal([]byte(event.Payload), &payloadData); err == nil {
					for k, v := range payloadData {
						payload[k] = v
					}
				}
			}
			r.broadcaster.PublishHatEvent(hatEventType, event.SessionID, taskID, projectID, payload)
		}
	}

	return r.Route(event, currentHat)
}

// topicToHatEvent maps internal event topics to hat event types for broadcasting
func topicToHatEvent(topic string) string {
	switch topic {
	case TopicPlanComplete:
		return realtime.EventHatPlanComplete
	case TopicDesignComplete:
		return realtime.EventHatDesignComplete
	case TopicImplementationDone:
		return realtime.EventHatImplementationDone
	case TopicReviewApproved:
		return realtime.EventHatReviewApproved
	case TopicReviewRejected:
		return realtime.EventHatReviewRejected
	case TopicTaskBlocked:
		return realtime.EventHatTaskBlocked
	case TopicResolved:
		return realtime.EventHatResolved
	default:
		return ""
	}
}

// GetEventHistory returns the event history for a session
func (r *EventRouter) GetEventHistory(sessionID string) ([]*db.Event, error) {
	if r.db == nil {
		return nil, nil
	}
	return r.db.ListEventsBySession(sessionID)
}
