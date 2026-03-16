package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/pkg/models"
)

type archivedScan struct {
	ID              string          `json:"scan_id"`
	DeviceID        string          `json:"device_id,omitempty"`
	Tool            string          `json:"tool,omitempty"`
	ConversationID  string          `json:"conversation_id,omitempty"`
	SessionID       string          `json:"session_id,omitempty"`
	StartTime       time.Time       `json:"start_time"`
	EndTime         time.Time       `json:"end_time"`
	DurationMs      int64           `json:"duration_ms"`
	TotalTokens     int             `json:"total_tokens"`
	InputTokens     int             `json:"input_tokens"`
	OutputTokens    int             `json:"output_tokens"`
	ThinkingTokens  int             `json:"thinking_tokens"`
	LLMCalls        int             `json:"llm_calls"`
	ToolCalls       int             `json:"tool_calls"`
	EstimatedCost   float64         `json:"estimated_cost"`
	EventsHash      string          `json:"events_hash"`
	EventCount      int             `json:"event_count"`
	EventTypeCounts map[string]int  `json:"event_type_counts,omitempty"`
	ArchivedAt      time.Time       `json:"archived_at"`
	Events          []archivedEvent `json:"events,omitempty"`
}

type archivedEvent struct {
	HookType       string    `json:"hook_type"`
	Timestamp      time.Time `json:"timestamp"`
	SessionID      string    `json:"session_id,omitempty"`
	Model          string    `json:"model,omitempty"`
	Tool           string    `json:"tool,omitempty"`
	ToolName       string    `json:"tool_name,omitempty"`
	FilePath       string    `json:"file_path,omitempty"`
	InputTokens    int       `json:"input_tokens,omitempty"`
	OutputTokens   int       `json:"output_tokens,omitempty"`
	ThinkingTokens int       `json:"thinking_tokens,omitempty"`
	DurationMs     int       `json:"duration_ms,omitempty"`
	ContentHash    string    `json:"content_hash,omitempty"`
}

func archiveScan(scan *models.Scan, cfg *config.Config) error {
	if !cfg.Local.Archive.Enabled {
		return nil
	}

	archiveDir := cfg.Local.Archive.Path
	if archiveDir == "" {
		if dataDir, err := config.GetDataDir(); err == nil {
			archiveDir = filepath.Join(dataDir, "archive")
		} else {
			return fmt.Errorf("failed to determine data directory: %w", err)
		}
	}
	archiveDir = os.ExpandEnv(archiveDir)

	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return err
	}

	archived := createarchivedScan(scan, cfg)

	if err := validateScanID(scan.ID); err != nil {
		return err
	}

	filename := filepath.Join(archiveDir, scan.ID+".json")
	data, err := json.MarshalIndent(archived, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0600)
}

func createarchivedScan(scan *models.Scan, cfg *config.Config) *archivedScan {
	eventTypeCounts := make(map[string]int)
	for _, e := range scan.Events {
		eventTypeCounts[string(e.HookType)]++
	}

	eventsHash := hashEvents(scan.Events)

	archived := &archivedScan{
		ID:              scan.ID,
		DeviceID:        scan.DeviceID,
		Tool:            scan.Tool,
		ConversationID:  scan.ConversationID,
		SessionID:       "",
		StartTime:       scan.StartTime,
		EndTime:         scan.EndTime,
		DurationMs:      scan.EndTime.Sub(scan.StartTime).Milliseconds(),
		TotalTokens:     scan.TotalTokens,
		InputTokens:     scan.InputTokens,
		OutputTokens:    scan.OutputTokens,
		ThinkingTokens:  scan.ThinkingTokens,
		LLMCalls:        scan.LLMCalls,
		ToolCalls:       scan.ToolCalls,
		EstimatedCost:   scan.EstimatedCost,
		EventsHash:      eventsHash,
		EventCount:      len(scan.Events),
		EventTypeCounts: eventTypeCounts,
		ArchivedAt:      time.Now().UTC(),
	}

	if len(scan.Events) > 0 {
		archived.SessionID = scan.Events[0].SessionID
	}

	if cfg.Local.Archive.IncludeEvents {
		archived.Events = redactEvents(scan.Events, cfg.Local.Archive.Redacted)
	}

	return archived
}

func redactEvents(events []models.Event, redacted bool) []archivedEvent {
	archived := make([]archivedEvent, len(events))
	for i, e := range events {
		ae := archivedEvent{
			HookType:       string(e.HookType),
			Timestamp:      e.Timestamp,
			SessionID:      e.SessionID,
			Model:          e.Model,
			Tool:           e.Tool,
			ToolName:       e.ToolName,
			FilePath:       e.FilePath,
			InputTokens:    e.InputTokens,
			OutputTokens:   e.OutputTokens,
			ThinkingTokens: e.ThinkingTokens,
			DurationMs:     e.DurationMs,
		}

		if redacted {
			ae.ContentHash = hashContent(e.Prompt, e.Response, e.Thought, string(e.ToolOutput), e.CommandOutput)
		}

		archived[i] = ae
	}
	return archived
}

func hashEvents(events []models.Event) string {
	h := sha256.New()
	for _, e := range events {
		h.Write([]byte(string(e.HookType)))
		h.Write([]byte(e.Timestamp.Format(time.RFC3339Nano)))
		h.Write([]byte(e.ConversationID))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func hashContent(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
