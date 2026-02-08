package tools

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewBlockingContext(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "task-1")
	if bc.SessionID != "session-1" {
		t.Errorf("expected SessionID 'session-1', got %q", bc.SessionID)
	}
	if bc.QuestID != "quest-1" {
		t.Errorf("expected QuestID 'quest-1', got %q", bc.QuestID)
	}
	if bc.TaskID != "task-1" {
		t.Errorf("expected TaskID 'task-1', got %q", bc.TaskID)
	}
	if bc.Timeout != 5*time.Minute {
		t.Errorf("expected default timeout of 5m, got %v", bc.Timeout)
	}
	if bc.TestMode {
		t.Error("expected TestMode to be false")
	}
}

func TestNewTestBlockingContext(t *testing.T) {
	answerFunc := func(q QuestionOptions) BlockingToolAnswer {
		return BlockingToolAnswer{Answer: "test answer"}
	}
	bc := NewTestBlockingContext(answerFunc)
	if !bc.TestMode {
		t.Error("expected TestMode to be true")
	}
	if bc.TestAnswerFunc == nil {
		t.Error("expected TestAnswerFunc to be set")
	}
}

func TestBlockingContext_WaitForAnswer_TestMode(t *testing.T) {
	answerFunc := func(q QuestionOptions) BlockingToolAnswer {
		return BlockingToolAnswer{
			Answer:          q.Options[0].Label,
			SelectedIndices: []int{0},
			IsCustom:        false,
		}
	}
	bc := NewTestBlockingContext(answerFunc)

	question := QuestionOptions{
		Question: "What framework?",
		Options: []QuestionOption{
			{Label: "React", Description: "Frontend framework"},
			{Label: "Vue", Description: "Progressive framework"},
		},
	}

	answer, err := bc.WaitForAnswer(context.Background(), "call-1", question)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer.Answer != "React" {
		t.Errorf("expected answer 'React', got %q", answer.Answer)
	}
	if len(answer.SelectedIndices) != 1 || answer.SelectedIndices[0] != 0 {
		t.Errorf("expected SelectedIndices [0], got %v", answer.SelectedIndices)
	}
}

func TestBlockingContext_WaitForAnswer_DeliverAnswer(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second

	question := QuestionOptions{
		Question: "What framework?",
		Options: []QuestionOption{
			{Label: "React"},
		},
	}

	var wg sync.WaitGroup
	var answer BlockingToolAnswer
	var waitErr error

	// Start waiting in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		answer, waitErr = bc.WaitForAnswer(context.Background(), "call-1", question)
	}()

	// Give it a moment to register the pending question
	time.Sleep(50 * time.Millisecond)

	// Verify pending question is registered
	pending := bc.GetPendingQuestions()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending question, got %d", len(pending))
	}
	if pending[0].CallID != "call-1" {
		t.Errorf("expected CallID 'call-1', got %q", pending[0].CallID)
	}

	// Deliver the answer
	err := bc.DeliverAnswer(BlockingToolAnswer{
		Answer:          "React",
		SelectedIndices: []int{0},
	})
	if err != nil {
		t.Fatalf("failed to deliver answer: %v", err)
	}

	// Wait for the WaitForAnswer to complete
	wg.Wait()

	if waitErr != nil {
		t.Fatalf("WaitForAnswer returned error: %v", waitErr)
	}
	if answer.Answer != "React" {
		t.Errorf("expected answer 'React', got %q", answer.Answer)
	}
}

func TestBlockingContext_WaitForAnswer_Timeout(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 100 * time.Millisecond

	question := QuestionOptions{Question: "Test?"}

	_, err := bc.WaitForAnswer(context.Background(), "call-1", question)
	if err != ErrBlockingTimeout {
		t.Errorf("expected ErrBlockingTimeout, got %v", err)
	}
}

func TestBlockingContext_WaitForAnswer_Cancel(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second

	question := QuestionOptions{Question: "Test?"}

	var wg sync.WaitGroup
	var waitErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, waitErr = bc.WaitForAnswer(context.Background(), "call-1", question)
	}()

	// Give it a moment to start waiting
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	bc.Cancel()

	wg.Wait()

	if waitErr != ErrBlockingCancelled {
		t.Errorf("expected ErrBlockingCancelled, got %v", waitErr)
	}
}

func TestBlockingContext_WaitForAnswer_ContextCancelled(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	question := QuestionOptions{Question: "Test?"}

	var wg sync.WaitGroup
	var waitErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, waitErr = bc.WaitForAnswer(ctx, "call-1", question)
	}()

	// Give it a moment to start waiting
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	wg.Wait()

	if waitErr != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", waitErr)
	}
}

func TestBlockingContext_HasPending(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second

	if bc.HasPending() {
		t.Error("expected no pending questions initially")
	}

	// Start a wait in background
	go func() {
		_, _ = bc.WaitForAnswer(context.Background(), "call-1", QuestionOptions{Question: "Test?"})
	}()

	time.Sleep(50 * time.Millisecond)

	if !bc.HasPending() {
		t.Error("expected pending question")
	}

	bc.Cancel()
}

func TestBlockingToolRegistry(t *testing.T) {
	registry := NewBlockingToolRegistry()

	bc1 := NewBlockingContext("session-1", "quest-1", "")
	bc2 := NewBlockingContext("session-2", "quest-2", "")

	registry.Register(bc1)
	registry.Register(bc2)

	// Get should return the correct context
	got := registry.Get("session-1")
	if got != bc1 {
		t.Error("expected to get bc1")
	}

	got = registry.Get("session-2")
	if got != bc2 {
		t.Error("expected to get bc2")
	}

	got = registry.Get("session-3")
	if got != nil {
		t.Error("expected nil for unknown session")
	}

	// Unregister should remove and cancel
	registry.Unregister("session-1")
	got = registry.Get("session-1")
	if got != nil {
		t.Error("expected nil after unregister")
	}
}

func TestBlockingToolRegistry_DeliverAnswer(t *testing.T) {
	registry := NewBlockingToolRegistry()
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second
	registry.Register(bc)

	var wg sync.WaitGroup
	var answer BlockingToolAnswer
	var waitErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		answer, waitErr = bc.WaitForAnswer(context.Background(), "call-1", QuestionOptions{Question: "Test?"})
	}()

	time.Sleep(50 * time.Millisecond)

	err := registry.DeliverAnswer("session-1", BlockingToolAnswer{Answer: "Yes"})
	if err != nil {
		t.Fatalf("failed to deliver answer: %v", err)
	}

	wg.Wait()

	if waitErr != nil {
		t.Fatalf("WaitForAnswer returned error: %v", waitErr)
	}
	if answer.Answer != "Yes" {
		t.Errorf("expected answer 'Yes', got %q", answer.Answer)
	}
}

func TestBlockingToolRegistry_DeliverAnswer_UnknownSession(t *testing.T) {
	registry := NewBlockingToolRegistry()

	err := registry.DeliverAnswer("unknown-session", BlockingToolAnswer{Answer: "Yes"})
	if err == nil {
		t.Error("expected error for unknown session")
	}
}

func TestFormatQuestionResult(t *testing.T) {
	answer := BlockingToolAnswer{
		Answer:          "React",
		SelectedIndices: []int{0, 2},
		IsCustom:        false,
	}

	result := FormatQuestionResult(answer)
	expected := `{"answer":"React","is_custom":false,"selected_indices":[0,2]}`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBlockingContext_CancelledAnswer(t *testing.T) {
	bc := NewBlockingContext("session-1", "quest-1", "")
	bc.Timeout = 5 * time.Second

	var wg sync.WaitGroup
	var waitErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, waitErr = bc.WaitForAnswer(context.Background(), "call-1", QuestionOptions{Question: "Test?"})
	}()

	time.Sleep(50 * time.Millisecond)

	// Deliver a cancelled answer
	err := bc.DeliverAnswer(BlockingToolAnswer{Cancelled: true})
	if err != nil {
		t.Fatalf("failed to deliver answer: %v", err)
	}

	wg.Wait()

	if waitErr != ErrBlockingCancelled {
		t.Errorf("expected ErrBlockingCancelled, got %v", waitErr)
	}
}
