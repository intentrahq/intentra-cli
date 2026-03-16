package scanner

import (
	"testing"
	"time"

	"github.com/intentrahq/intentra-cli/pkg/models"
)

func TestAggregateEvents(t *testing.T) {
	events := []models.Event{
		{NormalizedType: "before_prompt", ConversationID: "conv-1", Timestamp: time.Now()},
		{NormalizedType: "after_response", ConversationID: "conv-1", Timestamp: time.Now().Add(time.Second)},
		{NormalizedType: "stop", ConversationID: "conv-1", Timestamp: time.Now().Add(2 * time.Second)},
	}

	scans := AggregateEvents(events)
	if len(scans) != 1 {
		t.Fatalf("Expected 1 scan, got %d", len(scans))
	}
	if len(scans[0].Events) != 3 {
		t.Errorf("Expected 3 events in scan, got %d", len(scans[0].Events))
	}
}

func TestAggregateMultipleConversations(t *testing.T) {
	events := []models.Event{
		{NormalizedType: "before_prompt", ConversationID: "conv-1", Timestamp: time.Now()},
		{NormalizedType: "before_prompt", ConversationID: "conv-2", Timestamp: time.Now()},
		{NormalizedType: "stop", ConversationID: "conv-1", Timestamp: time.Now().Add(time.Second)},
		{NormalizedType: "stop", ConversationID: "conv-2", Timestamp: time.Now().Add(time.Second)},
	}

	scans := AggregateEvents(events)
	if len(scans) != 2 {
		t.Fatalf("Expected 2 scans, got %d", len(scans))
	}
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name     string
		tokens   int
		model    string
		tool     string
		expected float64
	}{
		{"claude-opus-4.5", 1000, "claude-opus-4.5-20250301", "", 0.011},
		{"claude-sonnet-4.5", 1000, "claude-sonnet-4.5-20250301", "", 0.0066},
		{"claude-haiku-4.5", 1000, "claude-haiku-4.5-20250301", "", 0.0022},
		{"claude-opus-4", 1000, "claude-opus-4-20250115", "", 0.033},
		{"claude-sonnet-4", 1000, "claude-sonnet-4-20250514", "", 0.0066},
		{"claude-3-5-sonnet", 1000, "claude-3-5-sonnet-20241022", "", 0.003},
		{"claude-3-opus", 1000, "claude-3-opus-20240229", "", 0.015},
		{"claude-3-haiku", 1000, "claude-3-haiku-20240307", "", 0.00025},
		{"gemini-2.5-pro", 1000, "gemini-2.5-pro-preview", "", 0.00388},
		{"gemini-2.0-flash", 1000, "gemini-2.0-flash-001", "", 0.00019},
		{"gemini-1.5-pro", 1000, "gemini-1.5-pro-latest", "", 0.00125},
		{"gpt-4o", 1000, "gpt-4o-2024-11-20", "", 0.005},
		{"gpt-4", 1000, "gpt-4-turbo", "", 0.03},
		{"gpt-3.5-turbo", 1000, "gpt-3.5-turbo-0125", "", 0.0005},
		{"o3", 1000, "o3-2025-04-16", "", 0.0038},
		{"windsurf 1.2x", 1000, "claude-sonnet-4.5-20250301", "windsurf", 0.0066 * 1.2},
		{"cursor 1.0x", 1000, "claude-sonnet-4.5-20250301", "cursor", 0.0066},
		{"copilot 1.0x", 1000, "claude-sonnet-4.5-20250301", "copilot", 0.0066},
		{"unknown model fallback", 1000, "some-unknown-model", "", 0.005},
		{"empty model fallback", 1000, "", "", 0.005},
		{"zero tokens", 0, "claude-sonnet-4.5", "", 0.0},
		{"large token count", 1000000, "claude-sonnet-4.5", "", 0.0066 * 1000},
		{"unknown tool 1.0x", 1000, "claude-sonnet-4.5", "unknown-tool", 0.0066},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got float64
			if tt.tool != "" {
				got = EstimateCost(tt.tokens, tt.model, tt.tool)
			} else {
				got = EstimateCost(tt.tokens, tt.model)
			}
			diff := got - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Errorf("EstimateCost(%d, %q, %q) = %f, want %f", tt.tokens, tt.model, tt.tool, got, tt.expected)
			}
		})
	}
}

func TestEstimateCost_LongestPrefixWins(t *testing.T) {
	costHighThinking := EstimateCost(1000, "claude-4.5-opus-high-thinking-20250301")
	costOpus45 := EstimateCost(1000, "claude-opus-4.5-20250301")
	if costHighThinking == costOpus45 {
		t.Errorf("expected different costs for high-thinking vs regular opus, both got %f", costHighThinking)
	}
}


func TestAggregateEvents_SkipsEmptyConversationID(t *testing.T) {
	events := []models.Event{
		{ConversationID: "", Timestamp: time.Now(), NormalizedType: "after_response"},
		{ConversationID: "conv-1", Timestamp: time.Now(), NormalizedType: "after_response"},
	}
	scans := AggregateEvents(events)
	if len(scans) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(scans))
	}
}

func TestAggregateFilesModified(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if result := AggregateFilesModified(nil); result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("counts edits", func(t *testing.T) {
		events := []models.Event{
			{FilePath: "/foo/bar.go", NormalizedType: "before_file_edit"},
			{FilePath: "/foo/bar.go", NormalizedType: "after_file_edit", OutputTokens: 150},
			{FilePath: "/foo/bar.go", NormalizedType: "after_file_edit", OutputTokens: 300},
		}
		result := AggregateFilesModified(events)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0]["edit_count"] != 2 {
			t.Errorf("edit_count = %v, want 2", result[0]["edit_count"])
		}
		if result[0]["is_new_file"] != false {
			t.Error("should not be new file (had before_file_edit)")
		}
	})

	t.Run("new file", func(t *testing.T) {
		events := []models.Event{
			{FilePath: "/foo/new.go", NormalizedType: "after_file_edit", OutputTokens: 100},
		}
		result := AggregateFilesModified(events)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0]["is_new_file"] != true {
			t.Error("should be new file (no before_file_edit)")
		}
	})
}

func TestSanitizePath(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if models.SanitizePath("") != "" {
			t.Error("expected empty for empty input")
		}
	})

	t.Run("non-home path unchanged", func(t *testing.T) {
		if result := models.SanitizePath("/tmp/foo"); result != "/tmp/foo" {
			t.Errorf("expected /tmp/foo, got %s", result)
		}
	})
}
