package worker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
	_ "modernc.org/sqlite"
)

// LocalDB is the worker's local database for storing objectives and activity.
// It uses application-level encryption for sensitive data.
//
// For full-database encryption, SQLCipher can be used as an alternative:
//
//	import _ "github.com/mutecomm/go-sqlcipher"
//	sql.Open("sqlite3", "worker.db?_pragma_key=passphrase")
//
// This requires CGO and the SQLCipher library.
type LocalDB struct {
	db        *sql.DB
	masterKey *crypto.MasterKey
	dbPath    string
}

// OpenLocalDB opens or creates a worker's local database.
// If masterKey is provided, sensitive data is encrypted at rest.
func OpenLocalDB(dbPath string, masterKey *crypto.MasterKey) (*LocalDB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open database with WAL mode
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	ldb := &LocalDB{
		db:        db,
		masterKey: masterKey,
		dbPath:    dbPath,
	}

	if err := ldb.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return ldb, nil
}

// Close closes the database connection.
func (ldb *LocalDB) Close() error {
	return ldb.db.Close()
}

// migrate runs database migrations.
func (ldb *LocalDB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS objectives (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			hat TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			base_branch TEXT,
			token_budget INTEGER,
			project_id TEXT,
			project_name TEXT,
			github_owner TEXT,
			github_repo TEXT,
			checklist TEXT,
			dispatched_at DATETIME,
			started_at DATETIME,
			completed_at DATETIME,
			hq_public_key TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			objective_id TEXT NOT NULL REFERENCES objectives(id),
			hat TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			iteration_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			ended_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS activity (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			objective_id TEXT NOT NULL,
			iteration INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			content TEXT,
			tokens_input INTEGER,
			tokens_output INTEGER,
			hat TEXT,
			synced INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_session ON activity(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_synced ON activity(synced)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_objective ON activity(objective_id)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			encrypted INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sync_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			synced_at DATETIME NOT NULL,
			ack_id TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_log_entity ON sync_log(entity_type, entity_id)`,
	}

	for _, migration := range migrations {
		if _, err := ldb.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// StoreObjective stores an objective received from HQ.
func (ldb *LocalDB) StoreObjective(payload *ObjectivePayload) error {
	checklistJSON := "[]"
	if len(payload.Objective.Checklist) > 0 {
		data, _ := json.Marshal(payload.Objective.Checklist)
		checklistJSON = string(data)
	}

	_, err := ldb.db.Exec(`
		INSERT INTO objectives (
			id, title, description, hat, status, base_branch, token_budget,
			project_id, project_name, github_owner, github_repo,
			checklist, dispatched_at, hq_public_key, created_at
		) VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		payload.Objective.ID,
		payload.Objective.Title,
		payload.Objective.Description,
		payload.Objective.Hat,
		payload.Objective.BaseBranch,
		payload.Objective.TokenBudget,
		payload.Project.ID,
		payload.Project.Name,
		payload.Project.GitHubOwner,
		payload.Project.GitHubRepo,
		checklistJSON,
		payload.DispatchedAt,
		payload.HQPublicKey,
		time.Now(),
	)
	return err
}

// GetObjective retrieves an objective by ID.
func (ldb *LocalDB) GetObjective(id string) (*Objective, error) {
	var obj Objective
	var checklistJSON string

	err := ldb.db.QueryRow(`
		SELECT id, title, description, hat, base_branch, token_budget, checklist
		FROM objectives WHERE id = ?
	`, id).Scan(&obj.ID, &obj.Title, &obj.Description, &obj.Hat, &obj.BaseBranch, &obj.TokenBudget, &checklistJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if checklistJSON != "" {
		_ = json.Unmarshal([]byte(checklistJSON), &obj.Checklist)
	}

	return &obj, nil
}

// UpdateObjectiveStatus updates an objective's status.
func (ldb *LocalDB) UpdateObjectiveStatus(id, status string) error {
	var completedAt interface{}
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = time.Now()
	}

	_, err := ldb.db.Exec(`
		UPDATE objectives SET status = ?, completed_at = ? WHERE id = ?
	`, status, completedAt, id)
	return err
}

// CreateSession creates a new session record.
func (ldb *LocalDB) CreateSession(id, objectiveID, hat string) error {
	_, err := ldb.db.Exec(`
		INSERT INTO sessions (id, objective_id, hat, status, created_at)
		VALUES (?, ?, ?, 'pending', ?)
	`, id, objectiveID, hat, time.Now())
	return err
}

// UpdateSessionStatus updates a session's status.
func (ldb *LocalDB) UpdateSessionStatus(id, status string) error {
	now := time.Now()
	var startedAt, endedAt interface{}

	if status == "running" {
		startedAt = now
	} else if status == "completed" || status == "failed" {
		endedAt = now
	}

	_, err := ldb.db.Exec(`
		UPDATE sessions SET status = ?, started_at = COALESCE(started_at, ?), ended_at = ?
		WHERE id = ?
	`, status, startedAt, endedAt, id)
	return err
}

// IncrementSessionIteration increments the iteration count for a session.
func (ldb *LocalDB) IncrementSessionIteration(id string) error {
	_, err := ldb.db.Exec(`
		UPDATE sessions SET iteration_count = iteration_count + 1 WHERE id = ?
	`, id)
	return err
}

// RecordActivity records a session activity event.
func (ldb *LocalDB) RecordActivity(event *ActivityEvent) error {
	_, err := ldb.db.Exec(`
		INSERT INTO activity (id, session_id, objective_id, iteration, event_type, content, tokens_input, tokens_output, hat, synced, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
	`, event.ID, event.SessionID, event.ObjectiveID, event.Iteration, event.EventType, event.Content, event.TokensInput, event.TokensOutput, event.Hat, event.CreatedAt)
	return err
}

// GetUnsyncedActivity returns all activity events that haven't been synced to HQ.
func (ldb *LocalDB) GetUnsyncedActivity(limit int) ([]*ActivityEvent, error) {
	rows, err := ldb.db.Query(`
		SELECT id, session_id, objective_id, iteration, event_type, content, tokens_input, tokens_output, hat, created_at
		FROM activity WHERE synced = 0 ORDER BY created_at ASC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*ActivityEvent
	for rows.Next() {
		var e ActivityEvent
		if err := rows.Scan(&e.ID, &e.SessionID, &e.ObjectiveID, &e.Iteration, &e.EventType, &e.Content, &e.TokensInput, &e.TokensOutput, &e.Hat, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// MarkActivitySynced marks activity events as synced.
func (ldb *LocalDB) MarkActivitySynced(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Build placeholders
	placeholders := ""
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}

	_, err := ldb.db.Exec(
		`UPDATE activity SET synced = 1 WHERE id IN (`+placeholders+`)`,
		args...,
	)
	return err
}

// StoreSecret stores an encrypted secret.
func (ldb *LocalDB) StoreSecret(key, value string) error {
	var storedValue string
	encrypted := 0

	if ldb.masterKey != nil {
		enc, err := ldb.masterKey.Encrypt([]byte(value))
		if err != nil {
			return fmt.Errorf("failed to encrypt secret: %w", err)
		}
		storedValue = enc
		encrypted = 1
	} else {
		storedValue = value
	}

	_, err := ldb.db.Exec(`
		INSERT INTO secrets (key, value, encrypted, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted
	`, key, storedValue, encrypted, time.Now())
	return err
}

// GetSecret retrieves and decrypts a secret.
func (ldb *LocalDB) GetSecret(key string) (string, error) {
	var value string
	var encrypted int

	err := ldb.db.QueryRow(`SELECT value, encrypted FROM secrets WHERE key = ?`, key).Scan(&value, &encrypted)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	if encrypted == 1 && ldb.masterKey != nil {
		decrypted, err := ldb.masterKey.Decrypt(value)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt secret: %w", err)
		}
		return string(decrypted), nil
	}

	return value, nil
}

// RecordSync records a successful sync to HQ.
func (ldb *LocalDB) RecordSync(entityType, entityID, ackID string) error {
	_, err := ldb.db.Exec(`
		INSERT INTO sync_log (entity_type, entity_id, synced_at, ack_id)
		VALUES (?, ?, ?, ?)
	`, entityType, entityID, time.Now(), ackID)
	return err
}

// GetObjectiveTokenUsage returns total tokens used for an objective.
func (ldb *LocalDB) GetObjectiveTokenUsage(objectiveID string) (input, output int, err error) {
	err = ldb.db.QueryRow(`
		SELECT COALESCE(SUM(tokens_input), 0), COALESCE(SUM(tokens_output), 0)
		FROM activity WHERE objective_id = ?
	`, objectiveID).Scan(&input, &output)
	return
}

// GetObjectiveIterationCount returns the total iteration count for an objective.
func (ldb *LocalDB) GetObjectiveIterationCount(objectiveID string) (int, error) {
	var count int
	err := ldb.db.QueryRow(`
		SELECT COALESCE(MAX(iteration), 0) FROM activity WHERE objective_id = ?
	`, objectiveID).Scan(&count)
	return count, err
}
