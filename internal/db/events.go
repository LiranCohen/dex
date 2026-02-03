// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Event represents a pub/sub event for hat coordination
type Event struct {
	ID        string
	SessionID string
	Topic     string
	Payload   sql.NullString
	SourceHat string
	CreatedAt time.Time
}

// CreateEvent inserts a new event record
func (db *DB) CreateEvent(sessionID, topic, payload, sourceHat string) (*Event, error) {
	event := &Event{
		ID:        NewPrefixedID("evt"),
		SessionID: sessionID,
		Topic:     topic,
		SourceHat: sourceHat,
		CreatedAt: time.Now(),
	}

	if payload != "" {
		event.Payload = sql.NullString{String: payload, Valid: true}
	}

	_, err := db.Exec(
		`INSERT INTO events (id, session_id, topic, payload, source_hat, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, event.Topic, event.Payload, event.SourceHat, event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	return event, nil
}

// ListEventsBySession returns all events for a session, ordered by creation time
func (db *DB) ListEventsBySession(sessionID string) ([]*Event, error) {
	rows, err := db.Query(
		`SELECT id, session_id, topic, payload, source_hat, created_at
		 FROM events WHERE session_id = ?
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID, &event.SessionID, &event.Topic,
			&event.Payload, &event.SourceHat, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

// GetEventsByTopic returns all events with a specific topic, ordered by creation time
func (db *DB) GetEventsByTopic(topic string) ([]*Event, error) {
	rows, err := db.Query(
		`SELECT id, session_id, topic, payload, source_hat, created_at
		 FROM events WHERE topic = ?
		 ORDER BY created_at ASC`,
		topic,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by topic: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID, &event.SessionID, &event.Topic,
			&event.Payload, &event.SourceHat, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

// GetLatestEventBySession returns the most recent event for a session
func (db *DB) GetLatestEventBySession(sessionID string) (*Event, error) {
	event := &Event{}
	err := db.QueryRow(
		`SELECT id, session_id, topic, payload, source_hat, created_at
		 FROM events WHERE session_id = ?
		 ORDER BY created_at DESC LIMIT 1`,
		sessionID,
	).Scan(
		&event.ID, &event.SessionID, &event.Topic,
		&event.Payload, &event.SourceHat, &event.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest event: %w", err)
	}
	return event, nil
}

// DeleteEventsBySession removes all events for a session
func (db *DB) DeleteEventsBySession(sessionID string) error {
	_, err := db.Exec(`DELETE FROM events WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete events: %w", err)
	}
	return nil
}
