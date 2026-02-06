package worker

import (
	"strings"
	"testing"
	"time"
)

func TestWorkerSession_NewWorkerSession(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/path/to/work")

	if session.ID != "sess-123" {
		t.Errorf("expected ID sess-123, got %s", session.ID)
	}
	if session.ObjectiveID != "obj-456" {
		t.Errorf("expected ObjectiveID obj-456, got %s", session.ObjectiveID)
	}
	if session.Hat != "explorer" {
		t.Errorf("expected Hat explorer, got %s", session.Hat)
	}
	if session.WorkDir != "/path/to/work" {
		t.Errorf("expected WorkDir /path/to/work, got %s", session.WorkDir)
	}
	if session.TaskID != "obj-456" {
		t.Errorf("expected TaskID obj-456, got %s", session.TaskID)
	}
	if session.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestWorkerSession_IterationTracking(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetIteration() != 0 {
		t.Errorf("expected initial iteration 0, got %d", session.GetIteration())
	}

	session.RecordIteration(100, 50)
	if session.GetIteration() != 1 {
		t.Errorf("expected iteration 1, got %d", session.GetIteration())
	}

	session.RecordIteration(200, 100)
	session.RecordIteration(300, 150)
	if session.GetIteration() != 3 {
		t.Errorf("expected iteration 3, got %d", session.GetIteration())
	}
}

func TestWorkerSession_TokenTracking(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	session.RecordIteration(100, 50)
	input, output := session.GetTokenUsage()
	if input != 100 || output != 50 {
		t.Errorf("expected tokens 100/50, got %d/%d", input, output)
	}

	session.RecordIteration(200, 100)
	input, output = session.GetTokenUsage()
	if input != 300 || output != 150 {
		t.Errorf("expected tokens 300/150, got %d/%d", input, output)
	}

	// Test TotalTokens
	total := session.TotalTokens()
	if total != 450 {
		t.Errorf("expected total tokens 450, got %d", total)
	}
}

func TestWorkerSession_BudgetTracking(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	session.SetBudgets(10000, 100, 60*time.Minute)

	// Check budget values are set
	if session.TokenBudget != 10000 {
		t.Errorf("expected TokenBudget 10000, got %d", session.TokenBudget)
	}
	if session.MaxIterations != 100 {
		t.Errorf("expected MaxIterations 100, got %d", session.MaxIterations)
	}
	if session.MaxRuntime != 60*time.Minute {
		t.Errorf("expected MaxRuntime 60m, got %v", session.MaxRuntime)
	}
}

func TestWorkerSession_Scratchpad(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetScratchpad() != "" {
		t.Error("expected empty scratchpad initially")
	}

	session.UpdateScratchpad("some notes")
	if session.GetScratchpad() != "some notes" {
		t.Errorf("expected scratchpad 'some notes', got '%s'", session.GetScratchpad())
	}

	session.UpdateScratchpad("updated notes")
	if session.GetScratchpad() != "updated notes" {
		t.Errorf("expected scratchpad 'updated notes', got '%s'", session.GetScratchpad())
	}
}

func TestWorkerSession_HatTracking(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetHat() != "explorer" {
		t.Errorf("expected hat explorer, got %s", session.GetHat())
	}

	session.UpdateHat("creator")
	if session.GetHat() != "creator" {
		t.Errorf("expected hat creator, got %s", session.GetHat())
	}
}

func TestWorkerSession_ChecklistTracking(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Initially no checklist items completed
	done, failed := session.GetChecklistStatus()
	if len(done) != 0 || len(failed) != 0 {
		t.Errorf("expected 0 done/failed items, got %d/%d", len(done), len(failed))
	}

	// Mark items done
	session.MarkChecklistDone("item1")
	session.MarkChecklistDone("item2")

	done, _ = session.GetChecklistStatus()
	if len(done) != 2 {
		t.Errorf("expected 2 done items, got %d", len(done))
	}

	// Mark item failed
	session.MarkChecklistFailed("item3")

	_, failed = session.GetChecklistStatus()
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(failed))
	}

	// Verify specific items
	found := false
	done, _ = session.GetChecklistStatus()
	for _, item := range done {
		if item == "item1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected item1 in done list")
	}
}

func TestWorkerSession_QualityGateAttempts(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetQualityGateAttempts() != 0 {
		t.Errorf("expected 0 quality gate attempts initially, got %d", session.GetQualityGateAttempts())
	}

	session.IncrementQualityGateAttempts()
	if session.GetQualityGateAttempts() != 1 {
		t.Errorf("expected 1 quality gate attempt, got %d", session.GetQualityGateAttempts())
	}

	session.IncrementQualityGateAttempts()
	session.IncrementQualityGateAttempts()
	if session.GetQualityGateAttempts() != 3 {
		t.Errorf("expected 3 quality gate attempts, got %d", session.GetQualityGateAttempts())
	}
}

func TestWorkerSession_Runtime(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Runtime should be positive
	runtime := session.Runtime()
	if runtime < 0 {
		t.Errorf("expected positive runtime, got %v", runtime)
	}

	// After sleeping, runtime should increase
	time.Sleep(10 * time.Millisecond)
	runtime2 := session.Runtime()
	if runtime2 <= runtime {
		t.Errorf("expected runtime to increase, got %v -> %v", runtime, runtime2)
	}
}

func TestWorkerSession_Concurrency(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Test concurrent access
	done := make(chan bool)

	// Concurrent iteration records
	for i := 0; i < 10; i++ {
		go func() {
			session.RecordIteration(100, 50)
			done <- true
		}()
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = session.GetIteration()
			_, _ = session.GetTokenUsage()
			_ = session.GetScratchpad()
			_ = session.GetHat()
			done <- true
		}()
	}

	// Concurrent hat updates
	for i := 0; i < 10; i++ {
		go func() {
			session.UpdateHat("creator")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 30; i++ {
		<-done
	}

	// Verify final state
	if session.GetIteration() != 10 {
		t.Errorf("expected 10 iterations, got %d", session.GetIteration())
	}

	input, output := session.GetTokenUsage()
	if input != 1000 || output != 500 {
		t.Errorf("expected tokens 1000/500, got %d/%d", input, output)
	}
}

func TestWorkerSession_RecordHatTransition(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Initial state
	if session.GetHat() != "explorer" {
		t.Errorf("expected initial hat explorer, got %s", session.GetHat())
	}
	if session.GetTransitionCount() != 0 {
		t.Errorf("expected 0 transitions initially, got %d", session.GetTransitionCount())
	}

	// First transition
	session.RecordHatTransition("explorer", "planner", "plan.complete")

	if session.GetHat() != "planner" {
		t.Errorf("expected hat planner after transition, got %s", session.GetHat())
	}
	if session.GetTransitionCount() != 1 {
		t.Errorf("expected 1 transition, got %d", session.GetTransitionCount())
	}
	if session.PreviousHat != "explorer" {
		t.Errorf("expected PreviousHat explorer, got %s", session.PreviousHat)
	}

	// Second transition
	session.RecordHatTransition("planner", "creator", "design.complete")

	if session.GetHat() != "creator" {
		t.Errorf("expected hat creator after transition, got %s", session.GetHat())
	}
	if session.GetTransitionCount() != 2 {
		t.Errorf("expected 2 transitions, got %d", session.GetTransitionCount())
	}
	if session.PreviousHat != "planner" {
		t.Errorf("expected PreviousHat planner, got %s", session.PreviousHat)
	}

	// Check history
	history := session.GetHatHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Hat != "planner" {
		t.Errorf("expected first history entry to be planner, got %s", history[0].Hat)
	}
	if history[0].Event != "design.complete" {
		t.Errorf("expected first history event to be design.complete, got %s", history[0].Event)
	}
	if history[1].Hat != "creator" {
		t.Errorf("expected second history entry to be creator, got %s", history[1].Hat)
	}
}

func TestWorkerSession_HatVisitCount(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Initial count - no history yet but current hat counts
	if count := session.HatVisitCount("explorer"); count != 1 {
		t.Errorf("expected 1 visit to explorer initially, got %d", count)
	}
	if count := session.HatVisitCount("creator"); count != 0 {
		t.Errorf("expected 0 visits to creator initially, got %d", count)
	}

	// After transition
	session.RecordHatTransition("explorer", "planner", "plan.complete")
	session.RecordHatTransition("planner", "creator", "design.complete")
	session.RecordHatTransition("creator", "critic", "implementation.done")
	session.RecordHatTransition("critic", "creator", "review.rejected") // Back to creator

	// Creator should have been visited twice now
	if count := session.HatVisitCount("creator"); count != 2 {
		t.Errorf("expected 2 visits to creator, got %d", count)
	}
	if count := session.HatVisitCount("planner"); count != 1 {
		t.Errorf("expected 1 visit to planner, got %d", count)
	}
	if count := session.HatVisitCount("critic"); count != 1 {
		t.Errorf("expected 1 visit to critic, got %d", count)
	}
}

func TestWorkerSession_BuildHandoffContext(t *testing.T) {
	t.Run("No previous hat", func(t *testing.T) {
		session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
		ctx := session.BuildHandoffContext()
		if ctx != "" {
			t.Errorf("expected empty context with no previous hat, got %q", ctx)
		}
	})

	t.Run("With previous hat", func(t *testing.T) {
		session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
		session.RecordHatTransition("explorer", "planner", "plan.complete")

		ctx := session.BuildHandoffContext()
		if ctx == "" {
			t.Error("expected non-empty context after transition")
		}
		if !strings.Contains(ctx, "explorer") {
			t.Error("expected context to mention previous hat 'explorer'")
		}
	})

	t.Run("With scratchpad", func(t *testing.T) {
		session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
		session.UpdateScratchpad("Important notes about the task")
		session.RecordHatTransition("explorer", "planner", "plan.complete")

		ctx := session.BuildHandoffContext()
		if !strings.Contains(ctx, "Important notes") {
			t.Error("expected context to include scratchpad content")
		}
	})

	t.Run("With multiple transitions", func(t *testing.T) {
		session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
		session.RecordHatTransition("explorer", "planner", "plan.complete")
		session.RecordHatTransition("planner", "creator", "design.complete")
		session.RecordHatTransition("creator", "critic", "implementation.done")

		ctx := session.BuildHandoffContext()
		if !strings.Contains(ctx, "Hat Progression") {
			t.Error("expected context to include hat progression")
		}
		if !strings.Contains(ctx, "planner") {
			t.Error("expected context to mention planner in progression")
		}
	})
}

func TestWorkerSession_SetTransitionCount(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetTransitionCount() != 0 {
		t.Errorf("expected 0 transitions initially, got %d", session.GetTransitionCount())
	}

	session.SetTransitionCount(5)
	if session.GetTransitionCount() != 5 {
		t.Errorf("expected 5 transitions after set, got %d", session.GetTransitionCount())
	}

	session.SetTransitionCount(0)
	if session.GetTransitionCount() != 0 {
		t.Errorf("expected 0 transitions after reset, got %d", session.GetTransitionCount())
	}
}

func TestWorkerSession_RestoreHatHistory(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Create history to restore
	now := time.Now()
	history := []HatVisit{
		{Hat: "explorer", StartedAt: now.Add(-10 * time.Minute), EndedAt: now.Add(-8 * time.Minute), Event: "plan.complete"},
		{Hat: "planner", StartedAt: now.Add(-8 * time.Minute), EndedAt: now.Add(-5 * time.Minute), Event: "design.complete"},
		{Hat: "creator", StartedAt: now.Add(-5 * time.Minute)},
	}

	session.RestoreHatHistory(history)

	restored := session.GetHatHistory()
	if len(restored) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(restored))
	}
	if restored[0].Hat != "explorer" {
		t.Errorf("expected first hat explorer, got %s", restored[0].Hat)
	}
	if restored[1].Event != "design.complete" {
		t.Errorf("expected second event design.complete, got %s", restored[1].Event)
	}
	if restored[2].Hat != "creator" {
		t.Errorf("expected third hat creator, got %s", restored[2].Hat)
	}

	// Verify it's a copy (modifying original shouldn't affect session)
	history[0].Hat = "modified"
	restored = session.GetHatHistory()
	if restored[0].Hat != "explorer" {
		t.Error("expected session history to be independent copy")
	}
}

func TestWorkerSession_RestoreTokenUsage(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	// Initial state
	input, output := session.GetTokenUsage()
	if input != 0 || output != 0 {
		t.Errorf("expected 0/0 tokens initially, got %d/%d", input, output)
	}

	// Restore tokens
	session.RestoreTokenUsage(5000, 2500)

	input, output = session.GetTokenUsage()
	if input != 5000 || output != 2500 {
		t.Errorf("expected 5000/2500 tokens after restore, got %d/%d", input, output)
	}

	// Total should reflect restored values
	if session.TotalTokens() != 7500 {
		t.Errorf("expected total 7500, got %d", session.TotalTokens())
	}

	// Further iterations should add to restored values
	session.RecordIteration(100, 50)
	input, output = session.GetTokenUsage()
	if input != 5100 || output != 2550 {
		t.Errorf("expected 5100/2550 after iteration, got %d/%d", input, output)
	}
}

func TestWorkerSession_RestoreIteration(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.GetIteration() != 0 {
		t.Errorf("expected 0 iterations initially, got %d", session.GetIteration())
	}

	session.RestoreIteration(25)
	if session.GetIteration() != 25 {
		t.Errorf("expected 25 iterations after restore, got %d", session.GetIteration())
	}

	// Further iterations should increment from restored count
	session.RecordIteration(100, 50)
	if session.GetIteration() != 26 {
		t.Errorf("expected 26 iterations after one more, got %d", session.GetIteration())
	}
}

func TestWorkerSession_SetPreviousHat(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")

	if session.PreviousHat != "" {
		t.Errorf("expected empty previous hat initially, got %q", session.PreviousHat)
	}

	session.SetPreviousHat("planner")
	if session.PreviousHat != "planner" {
		t.Errorf("expected previous hat planner, got %s", session.PreviousHat)
	}

	// BuildHandoffContext should work with manually set previous hat
	ctx := session.BuildHandoffContext()
	if !strings.Contains(ctx, "planner") {
		t.Error("expected handoff context to mention planner")
	}
}

func TestWorkerSession_FullRestoration(t *testing.T) {
	// Simulate a full session restoration after crash
	session := NewWorkerSession("sess-restored", "obj-456", "creator", "/work")

	// Restore all state
	now := time.Now()
	history := []HatVisit{
		{Hat: "explorer", StartedAt: now.Add(-20 * time.Minute), EndedAt: now.Add(-15 * time.Minute), Event: "plan.complete"},
		{Hat: "planner", StartedAt: now.Add(-15 * time.Minute), EndedAt: now.Add(-10 * time.Minute), Event: "design.complete"},
	}

	session.RestoreHatHistory(history)
	session.RestoreTokenUsage(10000, 5000)
	session.RestoreIteration(15)
	session.SetTransitionCount(2)
	session.SetPreviousHat("planner")
	session.UpdateScratchpad("Previous work notes")

	// Verify complete state
	if session.GetIteration() != 15 {
		t.Errorf("expected iteration 15, got %d", session.GetIteration())
	}
	if session.TotalTokens() != 15000 {
		t.Errorf("expected total tokens 15000, got %d", session.TotalTokens())
	}
	if session.GetTransitionCount() != 2 {
		t.Errorf("expected 2 transitions, got %d", session.GetTransitionCount())
	}
	if session.PreviousHat != "planner" {
		t.Errorf("expected previous hat planner, got %s", session.PreviousHat)
	}
	if len(session.GetHatHistory()) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(session.GetHatHistory()))
	}

	// Handoff context should include history
	ctx := session.BuildHandoffContext()
	if !strings.Contains(ctx, "planner") {
		t.Error("expected handoff context to mention planner")
	}
	if !strings.Contains(ctx, "Previous work notes") {
		t.Error("expected handoff context to include scratchpad")
	}
}
