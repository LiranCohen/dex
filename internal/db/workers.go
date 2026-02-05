package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// WorkerStatus represents the enrollment status of a worker.
type WorkerStatus string

const (
	WorkerStatusPending  WorkerStatus = "pending"  // Awaiting approval
	WorkerStatusActive   WorkerStatus = "active"   // Enrolled and active
	WorkerStatusInactive WorkerStatus = "inactive" // Temporarily disabled
	WorkerStatusRevoked  WorkerStatus = "revoked"  // Permanently revoked
)

// Worker represents an enrolled worker node.
type Worker struct {
	ID         string       `json:"id"`
	Hostname   string       `json:"hostname"`
	PublicKey  string       `json:"public_key"` // Base64-encoded NaCl public key
	Status     WorkerStatus `json:"status"`
	EnrolledAt *time.Time   `json:"enrolled_at,omitempty"`
	LastSeenAt *time.Time   `json:"last_seen_at,omitempty"`
	MeshIP     string       `json:"mesh_ip,omitempty"`
	Tags       []string     `json:"tags,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

// CreateWorker creates a new worker enrollment record.
// Initially the worker is in 'pending' status until approved.
func (db *DB) CreateWorker(worker *Worker) error {
	now := time.Now()
	worker.CreatedAt = now
	if worker.Status == "" {
		worker.Status = WorkerStatusPending
	}

	tagsJSON := "[]"
	if len(worker.Tags) > 0 {
		data, _ := json.Marshal(worker.Tags)
		tagsJSON = string(data)
	}

	_, err := db.Exec(`
		INSERT INTO workers (id, hostname, public_key, status, mesh_ip, tags, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, worker.ID, worker.Hostname, worker.PublicKey, worker.Status, worker.MeshIP, tagsJSON, now)
	if err != nil {
		return fmt.Errorf("failed to create worker: %w", err)
	}
	return nil
}

// GetWorker retrieves a worker by ID.
func (db *DB) GetWorker(id string) (*Worker, error) {
	worker := &Worker{}
	var tagsJSON string
	var enrolledAt, lastSeenAt sql.NullTime

	err := db.QueryRow(`
		SELECT id, hostname, public_key, status, enrolled_at, last_seen_at, mesh_ip, tags, created_at
		FROM workers WHERE id = ?
	`, id).Scan(
		&worker.ID, &worker.Hostname, &worker.PublicKey, &worker.Status,
		&enrolledAt, &lastSeenAt, &worker.MeshIP, &tagsJSON, &worker.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	if enrolledAt.Valid {
		worker.EnrolledAt = &enrolledAt.Time
	}
	if lastSeenAt.Valid {
		worker.LastSeenAt = &lastSeenAt.Time
	}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &worker.Tags)
	}

	return worker, nil
}

// GetWorkerByHostname retrieves a worker by hostname.
func (db *DB) GetWorkerByHostname(hostname string) (*Worker, error) {
	worker := &Worker{}
	var tagsJSON string
	var enrolledAt, lastSeenAt sql.NullTime

	err := db.QueryRow(`
		SELECT id, hostname, public_key, status, enrolled_at, last_seen_at, mesh_ip, tags, created_at
		FROM workers WHERE hostname = ?
	`, hostname).Scan(
		&worker.ID, &worker.Hostname, &worker.PublicKey, &worker.Status,
		&enrolledAt, &lastSeenAt, &worker.MeshIP, &tagsJSON, &worker.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get worker by hostname: %w", err)
	}

	if enrolledAt.Valid {
		worker.EnrolledAt = &enrolledAt.Time
	}
	if lastSeenAt.Valid {
		worker.LastSeenAt = &lastSeenAt.Time
	}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &worker.Tags)
	}

	return worker, nil
}

// ListWorkers returns all workers, optionally filtered by status.
func (db *DB) ListWorkers(status WorkerStatus) ([]*Worker, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = db.Query(`
			SELECT id, hostname, public_key, status, enrolled_at, last_seen_at, mesh_ip, tags, created_at
			FROM workers WHERE status = ? ORDER BY created_at DESC
		`, status)
	} else {
		rows, err = db.Query(`
			SELECT id, hostname, public_key, status, enrolled_at, last_seen_at, mesh_ip, tags, created_at
			FROM workers ORDER BY created_at DESC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list workers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workers []*Worker
	for rows.Next() {
		worker := &Worker{}
		var tagsJSON string
		var enrolledAt, lastSeenAt sql.NullTime

		if err := rows.Scan(
			&worker.ID, &worker.Hostname, &worker.PublicKey, &worker.Status,
			&enrolledAt, &lastSeenAt, &worker.MeshIP, &tagsJSON, &worker.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan worker: %w", err)
		}

		if enrolledAt.Valid {
			worker.EnrolledAt = &enrolledAt.Time
		}
		if lastSeenAt.Valid {
			worker.LastSeenAt = &lastSeenAt.Time
		}
		if tagsJSON != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &worker.Tags)
		}

		workers = append(workers, worker)
	}
	return workers, rows.Err()
}

// ListActiveWorkers returns all workers with 'active' status.
func (db *DB) ListActiveWorkers() ([]*Worker, error) {
	return db.ListWorkers(WorkerStatusActive)
}

// ApproveWorker changes a worker's status from pending to active.
func (db *DB) ApproveWorker(id string) error {
	now := time.Now()
	result, err := db.Exec(`
		UPDATE workers SET status = ?, enrolled_at = ? WHERE id = ? AND status = ?
	`, WorkerStatusActive, now, id, WorkerStatusPending)
	if err != nil {
		return fmt.Errorf("failed to approve worker: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("worker not found or not in pending status")
	}
	return nil
}

// RevokeWorker changes a worker's status to revoked.
func (db *DB) RevokeWorker(id string) error {
	result, err := db.Exec(`
		UPDATE workers SET status = ? WHERE id = ?
	`, WorkerStatusRevoked, id)
	if err != nil {
		return fmt.Errorf("failed to revoke worker: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("worker not found")
	}
	return nil
}

// UpdateWorkerLastSeen updates the last_seen_at timestamp for a worker.
func (db *DB) UpdateWorkerLastSeen(id string, meshIP string) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE workers SET last_seen_at = ?, mesh_ip = ? WHERE id = ?
	`, now, meshIP, id)
	if err != nil {
		return fmt.Errorf("failed to update worker last seen: %w", err)
	}
	return nil
}

// DeleteWorker permanently removes a worker record.
func (db *DB) DeleteWorker(id string) error {
	_, err := db.Exec(`DELETE FROM workers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete worker: %w", err)
	}
	return nil
}

// GetActiveWorkerPublicKeys returns a map of worker ID to public key for all active workers.
// This is used by HQ when encrypting payloads for workers.
func (db *DB) GetActiveWorkerPublicKeys() (map[string]string, error) {
	rows, err := db.Query(`
		SELECT id, public_key FROM workers WHERE status = ?
	`, WorkerStatusActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get worker public keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	keys := make(map[string]string)
	for rows.Next() {
		var id, pubKey string
		if err := rows.Scan(&id, &pubKey); err != nil {
			return nil, fmt.Errorf("failed to scan worker key: %w", err)
		}
		keys[id] = pubKey
	}
	return keys, rows.Err()
}
