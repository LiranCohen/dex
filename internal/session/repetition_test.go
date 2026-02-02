package session

import (
	"testing"
)

func TestToolCallSignature_Equals(t *testing.T) {
	tests := []struct {
		name   string
		a      ToolCallSignature
		b      ToolCallSignature
		equals bool
	}{
		{
			name:   "identical",
			a:      ToolCallSignature{Name: "read_file", Params: `{"path":"foo.txt"}`},
			b:      ToolCallSignature{Name: "read_file", Params: `{"path":"foo.txt"}`},
			equals: true,
		},
		{
			name:   "different name",
			a:      ToolCallSignature{Name: "read_file", Params: `{"path":"foo.txt"}`},
			b:      ToolCallSignature{Name: "write_file", Params: `{"path":"foo.txt"}`},
			equals: false,
		},
		{
			name:   "different params",
			a:      ToolCallSignature{Name: "read_file", Params: `{"path":"foo.txt"}`},
			b:      ToolCallSignature{Name: "read_file", Params: `{"path":"bar.txt"}`},
			equals: false,
		},
		{
			name:   "empty params",
			a:      ToolCallSignature{Name: "git_status", Params: ""},
			b:      ToolCallSignature{Name: "git_status", Params: ""},
			equals: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equals(tt.b); got != tt.equals {
				t.Errorf("Equals() = %v, want %v", got, tt.equals)
			}
		})
	}
}

func TestRepetitionInspector_AllowsDifferentCalls(t *testing.T) {
	ri := NewRepetitionInspector()

	calls := []ToolCallSignature{
		{Name: "read_file", Params: `{"path":"a.txt"}`},
		{Name: "read_file", Params: `{"path":"b.txt"}`},
		{Name: "write_file", Params: `{"path":"c.txt"}`},
		{Name: "git_status", Params: ""},
	}

	for _, call := range calls {
		allowed, reason := ri.Check(call)
		if !allowed {
			t.Errorf("Expected call %v to be allowed, got blocked: %s", call, reason)
		}
	}
}

func TestRepetitionInspector_BlocksAfterMaxRepetitions(t *testing.T) {
	ri := NewRepetitionInspectorWithConfig(3, 2) // Block after 3 consecutive

	call := ToolCallSignature{Name: "read_file", Params: `{"path":"same.txt"}`}

	// First 3 calls should be allowed
	for i := 1; i <= 3; i++ {
		allowed, _ := ri.Check(call)
		if !allowed {
			t.Errorf("Call %d should be allowed", i)
		}
	}

	// 4th call should be blocked
	allowed, reason := ri.Check(call)
	if allowed {
		t.Error("4th identical call should be blocked")
	}
	if reason == "" {
		t.Error("Expected reason for blocking")
	}
}

func TestRepetitionInspector_ResetsOnDifferentCall(t *testing.T) {
	ri := NewRepetitionInspectorWithConfig(3, 2)

	call1 := ToolCallSignature{Name: "read_file", Params: `{"path":"a.txt"}`}
	call2 := ToolCallSignature{Name: "read_file", Params: `{"path":"b.txt"}`}

	// 3 identical calls
	for i := 0; i < 3; i++ {
		ri.Check(call1)
	}

	// Different call resets the counter
	ri.Check(call2)

	// Now 3 more of the first call should be allowed
	for i := 1; i <= 3; i++ {
		allowed, _ := ri.Check(call1)
		if !allowed {
			t.Errorf("Call %d after reset should be allowed", i)
		}
	}
}

func TestRepetitionInspector_ShouldTerminateAfterMaxBlocks(t *testing.T) {
	ri := NewRepetitionInspectorWithConfig(2, 2) // Block after 2 consecutive, terminate after 2 blocks

	call := ToolCallSignature{Name: "stuck_tool", Params: `{}`}

	// Trigger first block (calls 1, 2 allowed, 3 blocked)
	ri.Check(call)
	ri.Check(call)
	ri.Check(call) // blocked

	if ri.ShouldTerminate() {
		t.Error("Should not terminate after first block")
	}

	// Reset and trigger second block
	ri.Reset()
	ri.Check(call)
	ri.Check(call)
	ri.Check(call) // blocked again

	if !ri.ShouldTerminate() {
		t.Error("Should terminate after second block")
	}
}

func TestRepetitionInspector_Reset(t *testing.T) {
	ri := NewRepetitionInspector()

	call := ToolCallSignature{Name: "read_file", Params: `{"path":"test.txt"}`}

	// Make some calls
	for i := 0; i < 3; i++ {
		ri.Check(call)
	}

	stats := ri.Stats()
	if stats.RepeatCount != 3 {
		t.Errorf("Expected repeat count 3, got %d", stats.RepeatCount)
	}

	// Reset
	ri.Reset()

	stats = ri.Stats()
	if stats.RepeatCount != 0 {
		t.Errorf("Expected repeat count 0 after reset, got %d", stats.RepeatCount)
	}
	if stats.LastTool != "" {
		t.Errorf("Expected empty last tool after reset, got %s", stats.LastTool)
	}

	// Call counts should still be there
	if stats.TotalCallCounts["read_file"] != 3 {
		t.Errorf("Expected call count preserved, got %d", stats.TotalCallCounts["read_file"])
	}
}

func TestRepetitionInspector_ResetAll(t *testing.T) {
	ri := NewRepetitionInspector()

	call := ToolCallSignature{Name: "read_file", Params: `{"path":"test.txt"}`}

	// Make some calls and trigger a block
	for i := 0; i < 10; i++ {
		ri.Check(call)
	}

	// ResetAll should clear everything
	ri.ResetAll()

	stats := ri.Stats()
	if stats.RepeatCount != 0 {
		t.Errorf("Expected repeat count 0, got %d", stats.RepeatCount)
	}
	if stats.BlockCount != 0 {
		t.Errorf("Expected block count 0, got %d", stats.BlockCount)
	}
	if len(stats.TotalCallCounts) != 0 {
		t.Errorf("Expected empty call counts, got %v", stats.TotalCallCounts)
	}
}

func TestRepetitionInspector_Stats(t *testing.T) {
	ri := NewRepetitionInspector()

	ri.Check(ToolCallSignature{Name: "read_file", Params: `{"path":"a.txt"}`})
	ri.Check(ToolCallSignature{Name: "read_file", Params: `{"path":"b.txt"}`})
	ri.Check(ToolCallSignature{Name: "write_file", Params: `{"path":"c.txt"}`})

	stats := ri.Stats()

	if stats.LastTool != "write_file" {
		t.Errorf("Expected last tool 'write_file', got %s", stats.LastTool)
	}
	if stats.TotalCallCounts["read_file"] != 2 {
		t.Errorf("Expected read_file count 2, got %d", stats.TotalCallCounts["read_file"])
	}
	if stats.TotalCallCounts["write_file"] != 1 {
		t.Errorf("Expected write_file count 1, got %d", stats.TotalCallCounts["write_file"])
	}
}
