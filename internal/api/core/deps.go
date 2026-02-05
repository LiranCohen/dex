// Package core contains shared types and dependencies for API handlers.
package core

import (
	"context"
	"sync"
	"time"

	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/session"
	"github.com/lirancohen/dex/internal/task"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// ChallengeEntry holds a challenge and its expiry time for auth
type ChallengeEntry struct {
	Challenge string
	ExpiresAt time.Time
}

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
	Planner        *planning.Planner
	QuestHandler   *quest.Handler
	Realtime       *realtime.Node        // Centrifuge realtime node
	Broadcaster    *realtime.Broadcaster // Publishes to both legacy and new systems
	MeshClient     *mesh.Client          // Campus mesh network client
	TokenConfig    *auth.TokenConfig
	BaseDir        string

	// Auth challenge storage (shared with auth handlers)
	Challenges   map[string]ChallengeEntry
	ChallengesMu *sync.RWMutex

	// Thread-safe accessors for dynamically reloadable services
	// These are closures that handle the mutex locking internally
	GetToolbelt   func() *toolbelt.Toolbelt
	GetGitHubApp  func() *github.AppManager
	GetGitHubSync func() *github.SyncService

	// Cross-handler callbacks for complex orchestration
	// These allow handlers to trigger operations that span multiple services
	StartTaskInternal         func(ctx context.Context, taskID string, baseBranch string) (*StartTaskResult, error)
	StartTaskWithInheritance  func(ctx context.Context, taskID string, inheritedWorktree string, predecessorHandoff string) (*StartTaskResult, error)
	HandleTaskUnblocking      func(ctx context.Context, completedTaskID string)
	GeneratePredecessorHandoff func(task *db.Task) string

	// GitHub client fetchers
	GetToolbeltGitHubClient func(ctx context.Context, login string) (*toolbelt.GitHubClient, error)

	// Validation helpers
	IsValidGitRepo      func(path string) bool
	IsValidProjectPath  func(path string) bool
}
