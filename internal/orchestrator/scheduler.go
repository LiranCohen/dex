// Package orchestrator provides task scheduling and session management for Poindexter
package orchestrator

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/liranmauda/dex/internal/db"
	"github.com/liranmauda/dex/internal/task"
)

// Default maximum number of parallel sessions
const DefaultMaxParallel = 25

// QueuedTask represents a task waiting in the priority queue
type QueuedTask struct {
	TaskID    string
	Priority  int       // 1-5, lower = higher priority
	CreatedAt time.Time // For FIFO within same priority
	index     int       // Heap index for heap.Interface
}

// RunningTask represents a currently executing task
type RunningTask struct {
	TaskID    string
	Priority  int
	StartedAt time.Time
}

// PriorityQueue implements heap.Interface for tasks
// Lower priority number = higher priority (1 is highest)
// Same priority: earlier created_at wins (FIFO)
type PriorityQueue []*QueuedTask

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Lower priority number means higher priority
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	// Same priority: earlier created wins (FIFO)
	return pq[i].CreatedAt.Before(pq[j].CreatedAt)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*QueuedTask)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	item.index = -1 // For safety
	*pq = old[0 : n-1]
	return item
}

// Scheduler manages task scheduling with priority-based ordering
type Scheduler struct {
	db          *db.DB
	taskService *task.Service

	mu          sync.Mutex
	readyQueue  *PriorityQueue          // Tasks in "ready" status waiting to run
	running     map[string]*RunningTask // Currently running tasks keyed by TaskID
	taskIndex   map[string]int          // Maps TaskID to queue index for O(1) lookup
	maxParallel int                     // Max concurrent (default 25)
}

// NewScheduler creates a scheduler with max parallel limit
func NewScheduler(database *db.DB, taskService *task.Service, maxParallel int) *Scheduler {
	if maxParallel <= 0 {
		maxParallel = DefaultMaxParallel
	}

	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	return &Scheduler{
		db:          database,
		taskService: taskService,
		readyQueue:  &pq,
		running:     make(map[string]*RunningTask),
		taskIndex:   make(map[string]int),
		maxParallel: maxParallel,
	}
}

// Enqueue adds a ready task to the queue
func (s *Scheduler) Enqueue(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already in queue
	if _, exists := s.taskIndex[taskID]; exists {
		return fmt.Errorf("task %s already in queue", taskID)
	}

	// Check if already running
	if _, exists := s.running[taskID]; exists {
		return fmt.Errorf("task %s already running", taskID)
	}

	// Get task details from DB
	t, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Verify task is in ready status
	if t.Status != db.TaskStatusReady {
		return fmt.Errorf("task %s is not ready (status: %s)", taskID, t.Status)
	}

	// Add to queue
	item := &QueuedTask{
		TaskID:    taskID,
		Priority:  t.Priority,
		CreatedAt: t.CreatedAt,
	}
	heap.Push(s.readyQueue, item)
	s.taskIndex[taskID] = item.index

	return nil
}

// Dequeue removes a task from the queue (e.g., if it becomes blocked)
func (s *Scheduler) Dequeue(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dequeueLocked(taskID)
}

// dequeueLocked removes a task from the queue without locking
// Must be called with mutex held
func (s *Scheduler) dequeueLocked(taskID string) {
	idx, exists := s.taskIndex[taskID]
	if !exists {
		return
	}

	// Remove from heap
	heap.Remove(s.readyQueue, idx)
	delete(s.taskIndex, taskID)

	// Update indices for remaining items
	s.rebuildIndex()
}

// rebuildIndex updates the taskIndex map after heap operations
// Must be called with mutex held
func (s *Scheduler) rebuildIndex() {
	s.taskIndex = make(map[string]int)
	for i, item := range *s.readyQueue {
		item.index = i
		s.taskIndex[item.TaskID] = i
	}
}

// Next returns the next task to run, or nil if none ready or at capacity
// Also handles preemption if high-priority task is waiting
// Returns (toRun, toPauseID) where toPauseID is set if preemption is needed
func (s *Scheduler) Next() (*QueuedTask, *string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readyQueue.Len() == 0 {
		return nil, nil
	}

	// Check if we have capacity
	if len(s.running) < s.maxParallel {
		// Pop highest priority task
		item := heap.Pop(s.readyQueue).(*QueuedTask)
		delete(s.taskIndex, item.TaskID)
		return item, nil
	}

	// At capacity - check if preemption is needed
	top := (*s.readyQueue)[0] // Peek without removing
	lowest := s.getLowestPriorityRunningLocked()

	// Preempt if waiting task has higher priority (lower number)
	if lowest != nil && top.Priority < lowest.Priority {
		item := heap.Pop(s.readyQueue).(*QueuedTask)
		delete(s.taskIndex, item.TaskID)
		return item, &lowest.TaskID
	}

	return nil, nil
}

// MarkRunning moves a task from ready queue to running map
func (s *Scheduler) MarkRunning(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already running
	if _, exists := s.running[taskID]; exists {
		return fmt.Errorf("task %s already running", taskID)
	}

	// Get task details for priority
	t, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Add to running map
	s.running[taskID] = &RunningTask{
		TaskID:    taskID,
		Priority:  t.Priority,
		StartedAt: time.Now(),
	}

	// Remove from queue if present (shouldn't be, but be safe)
	if _, exists := s.taskIndex[taskID]; exists {
		s.dequeueLocked(taskID)
	}

	return nil
}

// MarkComplete removes a task from running map
func (s *Scheduler) MarkComplete(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, taskID)
}

// MarkPaused removes a task from running map (for preemption)
// The task should be re-queued after its status is updated
func (s *Scheduler) MarkPaused(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, taskID)
}

// RunningCount returns number of currently running tasks
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.running)
}

// QueueSize returns number of tasks waiting in queue
func (s *Scheduler) QueueSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readyQueue.Len()
}

// GetLowestPriorityRunning returns the running task with lowest priority (for preemption)
func (s *Scheduler) GetLowestPriorityRunning() *RunningTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getLowestPriorityRunningLocked()
}

// getLowestPriorityRunningLocked finds the running task with lowest priority
// Must be called with mutex held
func (s *Scheduler) getLowestPriorityRunningLocked() *RunningTask {
	var lowest *RunningTask
	for _, rt := range s.running {
		if lowest == nil || rt.Priority > lowest.Priority {
			lowest = rt
		}
	}
	return lowest
}

// GetRunningTasks returns a copy of all currently running tasks
func (s *Scheduler) GetRunningTasks() []*RunningTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*RunningTask, 0, len(s.running))
	for _, rt := range s.running {
		// Return a copy to prevent external modification
		tasks = append(tasks, &RunningTask{
			TaskID:    rt.TaskID,
			Priority:  rt.Priority,
			StartedAt: rt.StartedAt,
		})
	}
	return tasks
}

// GetQueuedTasks returns a copy of all queued tasks in priority order
func (s *Scheduler) GetQueuedTasks() []*QueuedTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*QueuedTask, s.readyQueue.Len())
	for i, qt := range *s.readyQueue {
		tasks[i] = &QueuedTask{
			TaskID:    qt.TaskID,
			Priority:  qt.Priority,
			CreatedAt: qt.CreatedAt,
		}
	}
	return tasks
}

// LoadReadyTasks loads all ready tasks from the database into the queue
// This should be called on startup to rebuild the queue state
func (s *Scheduler) LoadReadyTasks() error {
	tasks, err := s.db.ListReadyTasks()
	if err != nil {
		return fmt.Errorf("failed to load ready tasks: %w", err)
	}

	loaded := 0
	failed := 0
	for _, t := range tasks {
		if err := s.Enqueue(t.ID); err != nil {
			failed++
			continue
		}
		loaded++
	}

	if failed > 0 {
		fmt.Printf("scheduler: loaded %d ready tasks, %d failed to enqueue\n", loaded, failed)
	}

	return nil
}

// LoadRunningTasks loads tasks with "running" status into the running map
// This recovers state after a restart
func (s *Scheduler) LoadRunningTasks() error {
	tasks, err := s.db.ListTasksByStatus(db.TaskStatusRunning)
	if err != nil {
		return fmt.Errorf("failed to load running tasks: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range tasks {
		startedAt := t.CreatedAt
		if t.StartedAt.Valid {
			startedAt = t.StartedAt.Time
		}
		s.running[t.ID] = &RunningTask{
			TaskID:    t.ID,
			Priority:  t.Priority,
			StartedAt: startedAt,
		}
	}

	return nil
}

// IsRunning checks if a task is currently running
func (s *Scheduler) IsRunning(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.running[taskID]
	return exists
}

// IsQueued checks if a task is in the ready queue
func (s *Scheduler) IsQueued(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.taskIndex[taskID]
	return exists
}
