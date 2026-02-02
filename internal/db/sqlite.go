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
		migrationSessionActivity,
		migrationPlanningSessions,
		migrationPlanningMessages,
		migrationTaskChecklists,
		migrationChecklistItems,
		migrationQuests,
		migrationQuestMessages,
		migrationQuestTemplates,
		migrationGitHubApp,
		migrationOnboardingProgress,
		migrationSecrets,
	}

	for i, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	// Run optional migrations that may fail if already applied
	// (e.g., adding columns to existing tables)
	optionalMigrations := []string{
		"ALTER TABLE webauthn_credentials ADD COLUMN backup_eligible INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE webauthn_credentials ADD COLUMN backup_state INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE tasks ADD COLUMN quest_id TEXT REFERENCES quests(id)",
		"ALTER TABLE tasks ADD COLUMN model TEXT DEFAULT 'sonnet'",
		// Session token/rate tracking (replaces tokens_used, dollars_used)
		"ALTER TABLE sessions ADD COLUMN input_tokens INTEGER DEFAULT 0",
		"ALTER TABLE sessions ADD COLUMN output_tokens INTEGER DEFAULT 0",
		"ALTER TABLE sessions ADD COLUMN input_rate REAL DEFAULT 3.0",
		"ALTER TABLE sessions ADD COLUMN output_rate REAL DEFAULT 15.0",
		// Quest tool calls support
		"ALTER TABLE quest_messages ADD COLUMN tool_calls TEXT",
		// Activity hat tracking
		"ALTER TABLE session_activity ADD COLUMN hat TEXT",
		// GitHub org ID for targeting installation
		"ALTER TABLE onboarding_progress ADD COLUMN github_org_id INTEGER",
		// Project remote tracking for fork workflows
		"ALTER TABLE projects ADD COLUMN remote_origin TEXT",
		"ALTER TABLE projects ADD COLUMN remote_upstream TEXT",
		// Task/Quest content stored in git files
		"ALTER TABLE tasks ADD COLUMN content_path TEXT",
		"ALTER TABLE quests ADD COLUMN conversation_path TEXT",
		// GitHub Issue sync for Quests (Tasks already have github_issue_number)
		"ALTER TABLE quests ADD COLUMN github_issue_number INTEGER",
	}
	for _, migration := range optionalMigrations {
		db.Exec(migration) // Ignore errors - column may already exist
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
	backup_eligible INTEGER NOT NULL DEFAULT 0,
	backup_state INTEGER NOT NULL DEFAULT 0,
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

const migrationSessionActivity = `
CREATE TABLE IF NOT EXISTS session_activity (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	iteration INTEGER NOT NULL,
	event_type TEXT NOT NULL,
	content TEXT,
	tokens_input INTEGER,
	tokens_output INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_session_activity_session ON session_activity(session_id);
CREATE INDEX IF NOT EXISTS idx_session_activity_iteration ON session_activity(session_id, iteration);
`

const migrationPlanningSessions = `
CREATE TABLE IF NOT EXISTS planning_sessions (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL DEFAULT 'processing',
	refined_prompt TEXT,
	original_prompt TEXT NOT NULL,
	pending_checklist TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	completed_at DATETIME,
	FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_planning_sessions_task ON planning_sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_planning_sessions_status ON planning_sessions(status);
`

const migrationPlanningMessages = `
CREATE TABLE IF NOT EXISTS planning_messages (
	id TEXT PRIMARY KEY,
	planning_session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (planning_session_id) REFERENCES planning_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_planning_messages_session ON planning_messages(planning_session_id);
`

const migrationTaskChecklists = `
CREATE TABLE IF NOT EXISTS task_checklists (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_task_checklists_task ON task_checklists(task_id);
`

const migrationChecklistItems = `
CREATE TABLE IF NOT EXISTS checklist_items (
	id TEXT PRIMARY KEY,
	checklist_id TEXT NOT NULL,
	parent_id TEXT,
	description TEXT NOT NULL,
	status TEXT DEFAULT 'pending',
	verification_notes TEXT,
	completed_at DATETIME,
	sort_order INTEGER DEFAULT 0,
	FOREIGN KEY (checklist_id) REFERENCES task_checklists(id) ON DELETE CASCADE,
	FOREIGN KEY (parent_id) REFERENCES checklist_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_checklist_items_checklist ON checklist_items(checklist_id);
CREATE INDEX IF NOT EXISTS idx_checklist_items_parent ON checklist_items(parent_id);
CREATE INDEX IF NOT EXISTS idx_checklist_items_status ON checklist_items(status);
`

const migrationQuests = `
CREATE TABLE IF NOT EXISTS quests (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	title TEXT,
	status TEXT NOT NULL DEFAULT 'active',
	model TEXT DEFAULT 'sonnet',
	auto_start_default INTEGER DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	completed_at DATETIME,
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_quests_project ON quests(project_id);
CREATE INDEX IF NOT EXISTS idx_quests_status ON quests(status);
`

const migrationQuestMessages = `
CREATE TABLE IF NOT EXISTS quest_messages (
	id TEXT PRIMARY KEY,
	quest_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (quest_id) REFERENCES quests(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_quest_messages_quest ON quest_messages(quest_id);
`

const migrationQuestTemplates = `
CREATE TABLE IF NOT EXISTS quest_templates (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	initial_prompt TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_quest_templates_project ON quest_templates(project_id);
`

const migrationGitHubApp = `
-- GitHub App configuration (singleton - only one row)
CREATE TABLE IF NOT EXISTS github_app_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	app_id INTEGER NOT NULL,
	app_slug TEXT NOT NULL,
	client_id TEXT NOT NULL,
	client_secret TEXT NOT NULL,
	private_key TEXT NOT NULL,
	webhook_secret TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- GitHub App installations (one per user/org that installs the app)
CREATE TABLE IF NOT EXISTS github_installations (
	id INTEGER PRIMARY KEY,
	account_id INTEGER NOT NULL,
	account_type TEXT NOT NULL,
	login TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_github_installations_login ON github_installations(login);
`

const migrationOnboardingProgress = `
-- Onboarding progress tracking (singleton - only one row)
CREATE TABLE IF NOT EXISTS onboarding_progress (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	current_step TEXT NOT NULL DEFAULT 'welcome',
	passkey_completed_at DATETIME,
	github_org_name TEXT,
	github_org_id INTEGER,
	github_app_completed_at DATETIME,
	github_install_completed_at DATETIME,
	anthropic_completed_at DATETIME,
	completed_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationSecrets = `
-- Secrets storage (replaces file-based secrets.json)
CREATE TABLE IF NOT EXISTS secrets (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
