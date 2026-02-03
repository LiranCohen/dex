package quest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAssignUniqueDraftIDs_UpdatesBlockedBy(t *testing.T) {
	// Test content with two drafts where draft-2 is blocked by draft-1
	content := `Some text before
OBJECTIVE_DRAFT:{"draft_id":"draft-1","title":"Create repo","blocked_by":[]}
Some text between
OBJECTIVE_DRAFT:{"draft_id":"draft-2","title":"Configure deployment","blocked_by":["draft-1"]}`

	result := assignUniqueDraftIDs(content)

	// Extract the two drafts from result
	marker := "OBJECTIVE_DRAFT:"
	parts := strings.Split(result, marker)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (before + 2 drafts), got %d", len(parts))
	}

	// Parse first draft
	jsonStr1, _ := extractJSONObject(parts[1])
	var draft1 map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr1), &draft1); err != nil {
		t.Fatalf("failed to parse draft1: %v", err)
	}

	// Parse second draft
	jsonStr2, _ := extractJSONObject(parts[2])
	var draft2 map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr2), &draft2); err != nil {
		t.Fatalf("failed to parse draft2: %v", err)
	}

	// Get the UUIDs
	uuid1 := draft1["draft_id"].(string)
	uuid2 := draft2["draft_id"].(string)

	// Verify draft_ids are UUIDs (not the original "draft-1", "draft-2")
	if uuid1 == "draft-1" {
		t.Error("draft1 draft_id was not replaced with UUID")
	}
	if uuid2 == "draft-2" {
		t.Error("draft2 draft_id was not replaced with UUID")
	}

	// Verify blocked_by in draft2 references the NEW UUID of draft1
	blockedBy, ok := draft2["blocked_by"].([]interface{})
	if !ok {
		t.Fatal("draft2 blocked_by is not an array")
	}
	if len(blockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockedBy))
	}
	blockerID := blockedBy[0].(string)
	if blockerID != uuid1 {
		t.Errorf("blocked_by should reference uuid1 (%s), got %s", uuid1, blockerID)
	}
	if blockerID == "draft-1" {
		t.Error("blocked_by still contains old draft-1 ID instead of UUID")
	}
}

func TestAssignUniqueDraftIDs_MultipleBlockers(t *testing.T) {
	// Test with multiple blockers
	content := `OBJECTIVE_DRAFT:{"draft_id":"a","title":"A","blocked_by":[]}
OBJECTIVE_DRAFT:{"draft_id":"b","title":"B","blocked_by":[]}
OBJECTIVE_DRAFT:{"draft_id":"c","title":"C","blocked_by":["a","b"]}`

	result := assignUniqueDraftIDs(content)

	// Extract drafts
	marker := "OBJECTIVE_DRAFT:"
	parts := strings.Split(result, marker)
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}

	var drafts []map[string]interface{}
	for i := 1; i < len(parts); i++ {
		jsonStr, _ := extractJSONObject(parts[i])
		var draft map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &draft); err != nil {
			t.Fatalf("failed to parse draft %d: %v", i, err)
		}
		drafts = append(drafts, draft)
	}

	// Get UUIDs for drafts a and b
	uuidA := drafts[0]["draft_id"].(string)
	uuidB := drafts[1]["draft_id"].(string)

	// Verify draft C's blocked_by references new UUIDs
	blockedBy := drafts[2]["blocked_by"].([]interface{})
	if len(blockedBy) != 2 {
		t.Fatalf("expected 2 blockers, got %d", len(blockedBy))
	}

	hasUuidA := false
	hasUuidB := false
	for _, b := range blockedBy {
		if b == uuidA {
			hasUuidA = true
		}
		if b == uuidB {
			hasUuidB = true
		}
	}
	if !hasUuidA {
		t.Error("blocked_by missing reference to draft A's new UUID")
	}
	if !hasUuidB {
		t.Error("blocked_by missing reference to draft B's new UUID")
	}
}

func TestAssignUniqueDraftIDs_NoBlockedBy(t *testing.T) {
	// Test draft without blocked_by field
	content := `OBJECTIVE_DRAFT:{"draft_id":"x","title":"X"}`

	result := assignUniqueDraftIDs(content)

	marker := "OBJECTIVE_DRAFT:"
	parts := strings.Split(result, marker)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}

	jsonStr, _ := extractJSONObject(parts[1])
	var draft map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &draft); err != nil {
		t.Fatalf("failed to parse draft: %v", err)
	}

	if draft["draft_id"] == "x" {
		t.Error("draft_id was not replaced with UUID")
	}
}

func TestAssignUniqueDraftIDs_PreservesOtherContent(t *testing.T) {
	content := `Before text
OBJECTIVE_DRAFT:{"draft_id":"test","title":"Test","blocked_by":[]}
After text with special chars: {} [] : " '`

	result := assignUniqueDraftIDs(content)

	if !strings.HasPrefix(result, "Before text\n") {
		t.Error("text before draft was not preserved")
	}
	if !strings.HasSuffix(result, `After text with special chars: {} [] : " '`) {
		t.Error("text after draft was not preserved")
	}
}
