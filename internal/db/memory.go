// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MemoryType represents the type of knowledge captured
type MemoryType string

const (
	MemoryArchitecture MemoryType = "architecture" // How code is organized
	MemoryDependency   MemoryType = "dependency"   // External deps and quirks
	MemoryDecision     MemoryType = "decision"     // Why something was chosen
	MemoryConstraint   MemoryType = "constraint"   // Limitations, requirements
	MemoryPattern      MemoryType = "pattern"      // How things are done here
	MemoryConvention   MemoryType = "convention"   // Style, naming, formatting
	MemoryPitfall      MemoryType = "pitfall"      // Things that don't work
	MemoryFix          MemoryType = "fix"          // Solutions to recurring problems
)

// Title returns a human-readable title for the memory type
func (t MemoryType) Title() string {
	titles := map[MemoryType]string{
		MemoryArchitecture: "Architecture",
		MemoryDependency:   "Dependencies",
		MemoryDecision:     "Decisions",
		MemoryConstraint:   "Constraints",
		MemoryPattern:      "Patterns",
		MemoryConvention:   "Conventions",
		MemoryPitfall:      "Pitfalls",
		MemoryFix:          "Fixes",
	}
	if title, ok := titles[t]; ok {
		return title
	}
	return string(t)
}

// IsValidMemoryType checks if a string is a valid memory type
func IsValidMemoryType(s string) bool {
	switch MemoryType(s) {
	case MemoryArchitecture, MemoryDependency, MemoryDecision, MemoryConstraint,
		MemoryPattern, MemoryConvention, MemoryPitfall, MemoryFix:
		return true
	}
	return false
}

// MemorySource indicates how the memory was created
type MemorySource string

const (
	SourceAutomatic MemorySource = "automatic" // Extracted automatically
	SourceExplicit  MemorySource = "explicit"  // Agent signaled MEMORY:
	SourceManual    MemorySource = "manual"    // Added via CLI/API
)

// Memory represents a learned piece of knowledge about a project
type Memory struct {
	ID        string
	ProjectID string
	Type      MemoryType
	Title     string
	Content   string

	// Relevance scoring
	Confidence float64
	Tags       []string
	FileRefs   []string

	// Provenance
	CreatedByHat       string
	CreatedByTaskID    sql.NullString
	CreatedBySessionID sql.NullString
	Source             MemorySource

	// Lifecycle
	CreatedAt  time.Time
	LastUsedAt sql.NullTime
	UseCount   int
	VerifiedAt sql.NullTime
}

// Confidence constants
const (
	InitialConfidenceExplicit  = 0.6
	InitialConfidenceAutomatic = 0.5
	InitialConfidenceManual    = 0.8

	UsageBoost    = 0.02
	MaxConfidence = 0.95
	DecayPerWeek  = 0.02
	MinConfidence = 0.1
)

// CreateMemory inserts a new memory into the database
func (db *DB) CreateMemory(m *Memory) error {
	tagsJSON, _ := json.Marshal(m.Tags)
	refsJSON, _ := json.Marshal(m.FileRefs)

	_, err := db.Exec(`
		INSERT INTO memories (
			id, project_id, type, title, content,
			confidence, tags, file_refs,
			created_by_hat, created_by_task_id, created_by_session_id, source,
			created_at, use_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.ID, m.ProjectID, m.Type, m.Title, m.Content,
		m.Confidence, string(tagsJSON), string(refsJSON),
		m.CreatedByHat, m.CreatedByTaskID, m.CreatedBySessionID, m.Source,
		m.CreatedAt, m.UseCount,
	)
	return err
}

// GetMemory retrieves a single memory by ID
func (db *DB) GetMemory(id string) (*Memory, error) {
	row := db.QueryRow(`
		SELECT id, project_id, type, title, content,
			confidence, tags, file_refs,
			created_by_hat, created_by_task_id, created_by_session_id, source,
			created_at, last_used_at, use_count, verified_at
		FROM memories WHERE id = ?
	`, id)

	return scanMemory(row)
}

// UpdateMemory updates an existing memory
func (db *DB) UpdateMemory(m *Memory) error {
	tagsJSON, _ := json.Marshal(m.Tags)
	refsJSON, _ := json.Marshal(m.FileRefs)

	_, err := db.Exec(`
		UPDATE memories SET
			type = ?, title = ?, content = ?,
			confidence = ?, tags = ?, file_refs = ?,
			last_used_at = ?, use_count = ?, verified_at = ?
		WHERE id = ?
	`,
		m.Type, m.Title, m.Content,
		m.Confidence, string(tagsJSON), string(refsJSON),
		m.LastUsedAt, m.UseCount, m.VerifiedAt,
		m.ID,
	)
	return err
}

// DeleteMemory removes a memory by ID
func (db *DB) DeleteMemory(id string) error {
	_, err := db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	return err
}

// ListMemories retrieves memories for a project with optional filters
func (db *DB) ListMemories(projectID string, memType *MemoryType, minConfidence float64) ([]Memory, error) {
	query := `
		SELECT id, project_id, type, title, content,
			confidence, tags, file_refs,
			created_by_hat, created_by_task_id, created_by_session_id, source,
			created_at, last_used_at, use_count, verified_at
		FROM memories
		WHERE project_id = ? AND confidence >= ?
	`
	args := []any{projectID, minConfidence}

	if memType != nil {
		query += ` AND type = ?`
		args = append(args, *memType)
	}

	query += ` ORDER BY confidence DESC, created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

// MemorySearchParams defines search parameters
type MemorySearchParams struct {
	Query            string
	Limit            int
	AfterDate        *time.Time
	BeforeDate       *time.Time
	ExcludeSessionID string // Prevent self-reference
}

// SearchMemories searches memories by title and content
func (db *DB) SearchMemories(projectID string, params MemorySearchParams) ([]Memory, error) {
	query := `
		SELECT id, project_id, type, title, content,
			confidence, tags, file_refs,
			created_by_hat, created_by_task_id, created_by_session_id, source,
			created_at, last_used_at, use_count, verified_at
		FROM memories
		WHERE project_id = ?
			AND (title LIKE ? OR content LIKE ?)
	`
	searchTerm := "%" + params.Query + "%"
	args := []any{projectID, searchTerm, searchTerm}

	if params.AfterDate != nil {
		query += ` AND created_at >= ?`
		args = append(args, *params.AfterDate)
	}
	if params.BeforeDate != nil {
		query += ` AND created_at <= ?`
		args = append(args, *params.BeforeDate)
	}
	if params.ExcludeSessionID != "" {
		query += ` AND (created_by_session_id IS NULL OR created_by_session_id != ?)`
		args = append(args, params.ExcludeSessionID)
	}

	query += ` ORDER BY confidence DESC, created_at DESC`

	if params.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, params.Limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

// MemoryContext defines context for relevance scoring
type MemoryContext struct {
	ProjectID        string
	CurrentHat       string
	CurrentSessionID string   // Exclude from search to prevent self-reference
	RelevantPaths    []string // Files being worked on
	TaskKeywords     []string // From task title/description
}

// GetRelevantMemories retrieves memories scored by relevance
func (db *DB) GetRelevantMemories(ctx MemoryContext, limit int) ([]Memory, error) {
	// Get candidate memories
	rows, err := db.Query(`
		SELECT id, project_id, type, title, content,
			confidence, tags, file_refs,
			created_by_hat, created_by_task_id, created_by_session_id, source,
			created_at, last_used_at, use_count, verified_at
		FROM memories
		WHERE project_id = ?
			AND confidence > 0.3
			AND (created_by_session_id IS NULL OR created_by_session_id != ?)
		ORDER BY confidence DESC
		LIMIT 50
	`, ctx.ProjectID, ctx.CurrentSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates, err := scanMemories(rows)
	if err != nil {
		return nil, err
	}

	// Score memories
	type scoredMemory struct {
		Memory Memory
		Score  float64
	}

	scored := make([]scoredMemory, 0, len(candidates))

	for _, m := range candidates {
		score := m.Confidence * 0.2 // Base score from confidence

		// Hat alignment: same or related hat gets boost
		if m.CreatedByHat == ctx.CurrentHat {
			score += 0.25
		} else if isRelatedHat(m.CreatedByHat, ctx.CurrentHat) {
			score += 0.1
		}

		// Path overlap: memories about files being touched
		for _, ref := range m.FileRefs {
			if pathOverlaps(ref, ctx.RelevantPaths) {
				score += 0.3
				break
			}
		}

		// Keyword match
		for _, tag := range m.Tags {
			for _, keyword := range ctx.TaskKeywords {
				if strings.Contains(strings.ToLower(tag), strings.ToLower(keyword)) {
					score += 0.15
					break
				}
			}
		}

		// Recency boost
		if m.LastUsedAt.Valid {
			daysSince := time.Since(m.LastUsedAt.Time).Hours() / 24
			recencyBoost := 0.1 - daysSince*0.002
			if recencyBoost > 0 {
				score += recencyBoost
			}
		}

		if score > 0.25 {
			scored = append(scored, scoredMemory{Memory: m, Score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Take top N and record usage
	result := make([]Memory, 0, limit)
	for i := 0; i < len(scored) && i < limit; i++ {
		result = append(result, scored[i].Memory)
		if err := db.RecordMemoryUsage(scored[i].Memory.ID); err != nil {
			fmt.Printf("warning: failed to record memory usage for %s: %v\n", scored[i].Memory.ID, err)
		}
	}

	return result, nil
}

// RecordMemoryUsage updates usage stats for a memory
func (db *DB) RecordMemoryUsage(memoryID string) error {
	_, err := db.Exec(`
		UPDATE memories
		SET use_count = use_count + 1,
			last_used_at = CURRENT_TIMESTAMP,
			confidence = MIN(?, confidence + ?)
		WHERE id = ?
	`, MaxConfidence, UsageBoost, memoryID)
	return err
}

// DecayUnusedMemories reduces confidence of memories not used recently
func (db *DB) DecayUnusedMemories() error {
	_, err := db.Exec(`
		UPDATE memories
		SET confidence = MAX(?, confidence - ?)
		WHERE last_used_at < datetime('now', '-7 days')
			AND confidence > ?
	`, MinConfidence, DecayPerWeek, MinConfidence)
	return err
}

// CleanupMemories removes low-value memories
func (db *DB) CleanupMemories() error {
	// Decay unused memories first
	if err := db.DecayUnusedMemories(); err != nil {
		fmt.Printf("warning: failed to decay unused memories: %v\n", err)
	}

	// Remove very low confidence memories that haven't been used
	_, err := db.Exec(`
		DELETE FROM memories
		WHERE confidence < 0.15
			AND use_count = 0
			AND created_at < datetime('now', '-30 days')
	`)
	return err
}

// Hat relationships for relevance scoring
var hatRelations = map[string][]string{
	"explorer": {"planner"},
	"planner":  {"creator", "explorer"},
	"creator":  {"critic", "editor", "planner"},
	"critic":   {"creator", "editor"},
	"editor":   {"creator", "critic"},
}

func isRelatedHat(memoryHat, currentHat string) bool {
	for _, related := range hatRelations[currentHat] {
		if memoryHat == related {
			return true
		}
	}
	return false
}

func pathOverlaps(ref string, paths []string) bool {
	// Simple prefix/glob matching
	for _, path := range paths {
		if strings.HasPrefix(path, strings.TrimSuffix(ref, "*")) {
			return true
		}
		if strings.HasPrefix(ref, strings.TrimSuffix(path, "*")) {
			return true
		}
	}
	return false
}

// scanMemory scans a single memory row
func scanMemory(row *sql.Row) (*Memory, error) {
	var m Memory
	var tagsJSON, refsJSON string

	err := row.Scan(
		&m.ID, &m.ProjectID, &m.Type, &m.Title, &m.Content,
		&m.Confidence, &tagsJSON, &refsJSON,
		&m.CreatedByHat, &m.CreatedByTaskID, &m.CreatedBySessionID, &m.Source,
		&m.CreatedAt, &m.LastUsedAt, &m.UseCount, &m.VerifiedAt,
	)
	if err != nil {
		return nil, err
	}

	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &m.Tags); err != nil {
			fmt.Printf("warning: failed to unmarshal memory tags for %s: %v\n", m.ID, err)
		}
	}
	if refsJSON != "" {
		if err := json.Unmarshal([]byte(refsJSON), &m.FileRefs); err != nil {
			fmt.Printf("warning: failed to unmarshal memory file_refs for %s: %v\n", m.ID, err)
		}
	}

	return &m, nil
}

// scanMemories scans multiple memory rows
func scanMemories(rows *sql.Rows) ([]Memory, error) {
	var memories []Memory

	for rows.Next() {
		var m Memory
		var tagsJSON, refsJSON string

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Type, &m.Title, &m.Content,
			&m.Confidence, &tagsJSON, &refsJSON,
			&m.CreatedByHat, &m.CreatedByTaskID, &m.CreatedBySessionID, &m.Source,
			&m.CreatedAt, &m.LastUsedAt, &m.UseCount, &m.VerifiedAt,
		)
		if err != nil {
			return nil, err
		}

		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &m.Tags); err != nil {
				fmt.Printf("warning: failed to unmarshal memory tags for %s: %v\n", m.ID, err)
			}
		}
		if refsJSON != "" {
			if err := json.Unmarshal([]byte(refsJSON), &m.FileRefs); err != nil {
				fmt.Printf("warning: failed to unmarshal memory file_refs for %s: %v\n", m.ID, err)
			}
		}

		memories = append(memories, m)
	}

	return memories, rows.Err()
}

