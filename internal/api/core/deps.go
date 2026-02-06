// Package core contains shared types and dependencies for API handlers.
package core

import (
	"context"

	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/forgejo"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/session"
	"github.com/lirancohen/dex/internal/task"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/worker"
)

// StartTaskResult contains the result of starting a task
type StartTaskResult struct {
	Task         *db.Task
	WorktreePath string
	SessionID    string
}

// Deps holds all dependencies needed by API handlers.
// This struct is passed to handler constructors to provide access to services.
type Deps struct {
	// Core services
	DB             *db.DB
	TaskService    *task.Service
	SessionManager *session.Manager
	GitService     *git.Service
	ForgejoManager *forgejo.Manager
	Planner        *planning.Planner
	QuestHandler   *quest.Handler
	Realtime       *realtime.Node            // Centrifuge realtime node
	Broadcaster    *realtime.Broadcaster     // Publishes to both legacy and new systems
	MeshClient     *mesh.Client              // Campus mesh network client
	WorkerManager  *worker.Manager           // Worker pool manager for distributed execution
	SecretsStore   *db.EncryptedSecretsStore // Encrypted secrets storage
	TokenConfig    *auth.TokenConfig
	BaseDir        string

	// Thread-safe accessors for dynamically reloadable services
	// These are closures that handle the mutex locking internally
	GetToolbelt func() *toolbelt.Toolbelt

	// Cross-handler callbacks for complex orchestration
	// These allow handlers to trigger operations that span multiple services
	StartTaskInternal          func(ctx context.Context, taskID string, baseBranch string) (*StartTaskResult, error)
	StartTaskWithInheritance   func(ctx context.Context, taskID string, inheritedWorktree string, predecessorHandoff string) (*StartTaskResult, error)
	HandleTaskUnblocking       func(ctx context.Context, completedTaskID string)
	GeneratePredecessorHandoff func(task *db.Task) string

	// Validation helpers
	IsValidGitRepo     func(path string) bool
	IsValidProjectPath func(path string) bool
}
