// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	*sql.DB
}

// Open creates or opens a SQLite database at the given path
func Open(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open database with WAL mode for better concurrent access
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Migrate runs all database migrations
func (db *DB) Migrate() error {
	migrations := []string{
		migrationUsers,
		migrationWebAuthnCredentials,
		migrationProjects,
		migrationTasks,
		migrationTaskDependencies,
		migrationSessions,
		migrationSessionCheckpoints,
		migrationApprovals,
	}

	for i, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

// Migration SQL statements

const migrationUsers = `
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	public_key TEXT UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME
);
`

const migrationWebAuthnCredentials = `
CREATE TABLE IF NOT EXISTS webauthn_credentials (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	credential_id BLOB NOT NULL UNIQUE,
	public_key BLOB NOT NULL,
	attestation_type TEXT NOT NULL DEFAULT 'none',
	aaguid BLOB,
	sign_count INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user ON webauthn_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_cred_id ON webauthn_credentials(credential_id);
`

const migrationProjects = `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	repo_path TEXT NOT NULL,
	github_owner TEXT,
	github_repo TEXT,
	default_branch TEXT DEFAULT 'main',
	services TEXT,  -- JSON blob for ProjectServices
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationTasks = `
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id),
	github_issue_number INTEGER,
	title TEXT NOT NULL,
	description TEXT,
	parent_id TEXT REFERENCES tasks(id),
	type TEXT NOT NULL DEFAULT 'task',  -- epic, feature, bug, task, chore
	hat TEXT,  -- Current hat assignment
	priority INTEGER DEFAULT 3,  -- 1-5 (1 highest)
	autonomy_level INTEGER DEFAULT 1,  -- 0-3
	status TEXT NOT NULL DEFAULT 'pending',  -- pending, blocked, ready, running, paused, quarantined, completed, cancelled
	base_branch TEXT DEFAULT 'main',
	worktree_path TEXT,
	branch_name TEXT,
	pr_number INTEGER,
	token_budget INTEGER,
	token_used INTEGER DEFAULT 0,
	time_budget_min INTEGER,
	time_used_min INTEGER DEFAULT 0,
	dollar_budget REAL,
	dollar_used REAL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
`

const migrationTaskDependencies = `
CREATE TABLE IF NOT EXISTS task_dependencies (
	blocker_id TEXT NOT NULL REFERENCES tasks(id),
	blocked_id TEXT NOT NULL REFERENCES tasks(id),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (blocker_id, blocked_id)
);

CREATE INDEX IF NOT EXISTS idx_task_deps_blocker ON task_dependencies(blocker_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_blocked ON task_dependencies(blocked_id);
`

const migrationSessions = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES tasks(id),
	hat TEXT NOT NULL,
	claude_session_id TEXT,
	status TEXT NOT NULL DEFAULT 'pending',  -- pending, running, paused, completed, failed
	worktree_path TEXT NOT NULL,
	iteration_count INTEGER DEFAULT 0,
	max_iterations INTEGER DEFAULT 100,
	completion_promise TEXT,
	tokens_used INTEGER DEFAULT 0,
	tokens_budget INTEGER,
	dollars_used REAL DEFAULT 0,
	dollars_budget REAL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	ended_at DATETIME,
	outcome TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
`

const migrationSessionCheckpoints = `
CREATE TABLE IF NOT EXISTS session_checkpoints (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	iteration INTEGER NOT NULL,
	state TEXT NOT NULL,  -- JSON blob of checkpoint state
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON session_checkpoints(session_id);
`

const migrationApprovals = `
CREATE TABLE IF NOT EXISTS approvals (
	id TEXT PRIMARY KEY,
	task_id TEXT REFERENCES tasks(id),
	session_id TEXT REFERENCES sessions(id),
	type TEXT NOT NULL,  -- commit, hat_transition, pr, merge, conflict_resolution
	title TEXT NOT NULL,
	description TEXT,
	data TEXT,  -- JSON blob with approval-specific data
	status TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	resolved_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);
CREATE INDEX IF NOT EXISTS idx_approvals_task ON approvals(task_id);
`
