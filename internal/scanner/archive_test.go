package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/pkg/models"
)

func TestRedactEvents(t *testing.T) {
	events := []models.Event{
		{
			HookType:       "afterResponse",
			Timestamp:      time.Now(),
			SessionID:      "sess-1",
			Model:          "claude-sonnet-4.5",
			Tool:           "cursor",
			FilePath:       "/foo/bar.go",
			InputTokens:    100,
			OutputTokens:   200,
			ThinkingTokens: 50,
			DurationMs:     300,
			Prompt:         "fix the bug",
			Response:       "here is the fix",
			Thought:        "thinking about it",
		},
	}

	t.Run("without redaction", func(t *testing.T) {
		result := redactEvents(events, false)
		if len(result) != 1 {
			t.Fatalf("expected 1 event, got %d", len(result))
		}
		if result[0].ContentHash != "" {
			t.Error("content_hash should be empty when not redacted")
		}
		if result[0].Model != "claude-sonnet-4.5" {
			t.Errorf("model = %q, want claude-sonnet-4.5", result[0].Model)
		}
		if result[0].InputTokens != 100 {
			t.Errorf("input_tokens = %d, want 100", result[0].InputTokens)
		}
	})

	t.Run("with redaction", func(t *testing.T) {
		result := redactEvents(events, true)
		if len(result) != 1 {
			t.Fatalf("expected 1 event, got %d", len(result))
		}
		if result[0].ContentHash == "" {
			t.Error("content_hash should be set when redacted")
		}
		if len(result[0].ContentHash) != 16 {
			t.Errorf("content_hash length = %d, want 16", len(result[0].ContentHash))
		}
	})

	t.Run("preserves metadata fields", func(t *testing.T) {
		result := redactEvents(events, true)
		ae := result[0]
		if ae.SessionID != "sess-1" {
			t.Errorf("session_id = %q, want sess-1", ae.SessionID)
		}
		if ae.FilePath != "/foo/bar.go" {
			t.Errorf("file_path = %q", ae.FilePath)
		}
		if ae.OutputTokens != 200 {
			t.Errorf("output_tokens = %d, want 200", ae.OutputTokens)
		}
	})
}

func TestHashEvents(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		events := []models.Event{
			{HookType: "afterResponse", Timestamp: ts, ConversationID: "c1"},
		}
		h1 := hashEvents(events)
		h2 := hashEvents(events)
		if h1 != h2 {
			t.Errorf("not deterministic: %q != %q", h1, h2)
		}
		if len(h1) != 16 {
			t.Errorf("length = %d, want 16", len(h1))
		}
	})

	t.Run("different events different hashes", func(t *testing.T) {
		ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		e1 := []models.Event{{HookType: "a", Timestamp: ts, ConversationID: "c1"}}
		e2 := []models.Event{{HookType: "b", Timestamp: ts, ConversationID: "c1"}}
		if hashEvents(e1) == hashEvents(e2) {
			t.Error("different events should produce different hashes")
		}
	})
}

func TestHashContent(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := hashContent("hello", "world")
		h2 := hashContent("hello", "world")
		if h1 != h2 {
			t.Errorf("not deterministic: %q != %q", h1, h2)
		}
		if len(h1) != 16 {
			t.Errorf("length = %d, want 16", len(h1))
		}
	})

	t.Run("different content different hashes", func(t *testing.T) {
		if hashContent("a") == hashContent("b") {
			t.Error("different content should produce different hashes")
		}
	})
}

func TestCreateArchivedScan(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	scan := &models.Scan{
		ID:             "scan-1",
		DeviceID:       "dev-1",
		Tool:           "cursor",
		ConversationID: "conv-1",
		StartTime:      start,
		EndTime:        end,
		TotalTokens:    1000,
		InputTokens:    600,
		OutputTokens:   400,
		LLMCalls:       5,
		ToolCalls:      3,
		EstimatedCost:  0.05,
		Events: []models.Event{
			{HookType: "afterResponse", SessionID: "sess-1", Timestamp: start},
			{HookType: "afterTool", SessionID: "sess-1", Timestamp: end},
		},
	}

	t.Run("basic fields", func(t *testing.T) {
		cfg := &config.Config{}
		archived := createarchivedScan(scan, cfg)

		if archived.ID != "scan-1" {
			t.Errorf("ID = %q", archived.ID)
		}
		if archived.DurationMs != 300000 {
			t.Errorf("duration_ms = %d, want 300000", archived.DurationMs)
		}
		if archived.EventCount != 2 {
			t.Errorf("event_count = %d, want 2", archived.EventCount)
		}
		if archived.SessionID != "sess-1" {
			t.Errorf("session_id = %q, want sess-1", archived.SessionID)
		}
		if archived.EventsHash == "" {
			t.Error("events_hash should not be empty")
		}
	})

	t.Run("events excluded by default", func(t *testing.T) {
		cfg := &config.Config{}
		archived := createarchivedScan(scan, cfg)
		if archived.Events != nil {
			t.Error("events should be nil when IncludeEvents is false")
		}
	})

	t.Run("events included when configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Local.Archive.IncludeEvents = true
		archived := createarchivedScan(scan, cfg)
		if len(archived.Events) != 2 {
			t.Errorf("expected 2 events, got %d", len(archived.Events))
		}
	})

	t.Run("events redacted when configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Local.Archive.IncludeEvents = true
		cfg.Local.Archive.Redacted = true
		scan.Events[0].Prompt = "secret prompt"
		archived := createarchivedScan(scan, cfg)
		if len(archived.Events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(archived.Events))
		}
		if archived.Events[0].ContentHash == "" {
			t.Error("content_hash should be set when redacted")
		}
	})

	t.Run("event type counts", func(t *testing.T) {
		cfg := &config.Config{}
		archived := createarchivedScan(scan, cfg)
		if archived.EventTypeCounts["afterResponse"] != 1 {
			t.Errorf("afterResponse count = %d, want 1", archived.EventTypeCounts["afterResponse"])
		}
		if archived.EventTypeCounts["afterTool"] != 1 {
			t.Errorf("afterTool count = %d, want 1", archived.EventTypeCounts["afterTool"])
		}
	})

	t.Run("no events gives empty session_id", func(t *testing.T) {
		emptyScan := &models.Scan{ID: "empty"}
		cfg := &config.Config{}
		archived := createarchivedScan(emptyScan, cfg)
		if archived.SessionID != "" {
			t.Errorf("session_id = %q, want empty", archived.SessionID)
		}
	})
}

func TestArchiveScan(t *testing.T) {
	t.Run("disabled returns nil", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Local.Archive.Enabled = false
		if err := archiveScan(&models.Scan{ID: "test"}, cfg); err != nil {
			t.Errorf("expected nil when disabled, got %v", err)
		}
	})

	t.Run("writes file to configured path", func(t *testing.T) {
		tmpDir := t.TempDir()
		archiveDir := filepath.Join(tmpDir, "archive")

		cfg := &config.Config{}
		cfg.Local.Archive.Enabled = true
		cfg.Local.Archive.Path = archiveDir

		scan := &models.Scan{
			ID:   "archive-test-1",
			Tool: "cursor",
		}

		if err := archiveScan(scan, cfg); err != nil {
			t.Fatalf("archiveScan failed: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(archiveDir, "archive-test-1.json"))
		if err != nil {
			t.Fatalf("failed to read archived file: %v", err)
		}

		var archived archivedScan
		if err := json.Unmarshal(data, &archived); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if archived.ID != "archive-test-1" {
			t.Errorf("ID = %q", archived.ID)
		}
	})

	t.Run("rejects invalid scan ID", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &config.Config{}
		cfg.Local.Archive.Enabled = true
		cfg.Local.Archive.Path = filepath.Join(tmpDir, "archive")

		scan := &models.Scan{ID: "../evil"}
		if err := archiveScan(scan, cfg); err == nil {
			t.Error("expected error for path traversal ID")
		}
	})
}
