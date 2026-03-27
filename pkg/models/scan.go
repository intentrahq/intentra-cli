package models

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"
)

// GenerateScanID produces a deterministic scan ID from a conversation ID and start time.
func GenerateScanID(conversationID string, startTime time.Time) string {
	hash := sha256.Sum256([]byte(conversationID + startTime.String()))
	return "scan_" + hex.EncodeToString(hash[:])[:12]
}

// ScanStatus represents the processing state of a scan.
type ScanStatus string

const (
	ScanStatusPending   ScanStatus = "pending"
	ScanStatusAnalyzing ScanStatus = "analyzing"
	ScanStatusReviewed  ScanStatus = "reviewed"
)

// ScanSource identifies the origin of a scan event.
type ScanSource struct {
	Tool      string `json:"tool,omitempty"`
	Event     string `json:"event,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// MCPToolCall represents aggregated usage of a single MCP server tool within a scan.
type MCPToolCall struct {
	ServerName    string  `json:"server_name"`
	ToolName      string  `json:"tool_name"`
	ServerURLHash string  `json:"server_url_hash,omitempty"`
	CallCount     int     `json:"call_count"`
	TotalDuration int     `json:"total_duration_ms"`
	EstimatedCost float64 `json:"estimated_cost"`
	ErrorCount    int     `json:"error_count"`
}

// Scan represents an aggregated conversation.
type Scan struct {
	ID             string      `json:"scan_id"`
	DeviceID       string      `json:"device_id"`
	Tool           string      `json:"tool,omitempty"`
	ConversationID string      `json:"conversation_id,omitempty"`
	GenerationID   string      `json:"generation_id,omitempty"`
	Model          string      `json:"model,omitempty"`
	Status         ScanStatus  `json:"status,omitempty"`
	StartTime      time.Time   `json:"start_time,omitempty"`
	EndTime        time.Time   `json:"end_time,omitempty"`
	Source         *ScanSource `json:"source,omitempty"`
	Events         []Event     `json:"events,omitempty"`

	TotalTokens    int     `json:"total_tokens"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	ThinkingTokens int     `json:"thinking_tokens"`
	LLMCalls       int     `json:"llm_calls"`
	ToolCalls      int     `json:"tool_calls"`
	EstimatedCost  float64 `json:"estimated_cost"`

	RawEvents []map[string]any `json:"raw_events,omitempty"`

	Fingerprint  string         `json:"fingerprint,omitempty"`
	FilesHash    string         `json:"files_hash,omitempty"`
	ActionCounts map[string]int `json:"action_counts,omitempty"`

	MCPToolUsage []MCPToolCall `json:"mcp_tool_usage,omitempty"`

	SessionEndReason  string `json:"session_end_reason,omitempty"`
	SessionDurationMs int64  `json:"session_duration_ms,omitempty"`

	RepoName      string           `json:"repo_name,omitempty"`
	RepoURLHash   string           `json:"repo_url_hash,omitempty"`
	BranchName    string           `json:"branch_name,omitempty"`
	CommitSHA     string           `json:"commit_sha,omitempty"`
	FilesModified []map[string]any `json:"files_modified,omitempty"`
}

// SendPayload is the JSON envelope written to a temp file by the hook handler
// and consumed by the __send subcommand.
type SendPayload struct {
	Action     string `json:"action"`
	Scan       *Scan  `json:"scan,omitempty"`
	ScanID     string `json:"scan_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

var (
	homeDirOnce sync.Once
	homeDir     string
)

func cachedHomeDir() string {
	homeDirOnce.Do(func() {
		homeDir, _ = os.UserHomeDir()
	})
	return homeDir
}

// SanitizePath replaces the home directory prefix with ~ to avoid storing absolute paths.
func SanitizePath(path string) string {
	if path == "" {
		return path
	}
	home := cachedHomeDir()
	if home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// BuildAPIPayload constructs the JSON-serializable map for sending a scan to the API.
// The deviceID parameter is the caller's device identifier.
// The richTraces parameter controls whether tool inputs/outputs and command content are included.
func (s *Scan) BuildAPIPayload(deviceID string, richTraces bool) map[string]any {
	durationMs := int64(0)
	if !s.EndTime.IsZero() && !s.StartTime.IsZero() {
		durationMs = s.EndTime.Sub(s.StartTime).Milliseconds()
	}

	sessionID := ""
	if s.Source != nil {
		sessionID = s.Source.SessionID
	}

	events := buildEventPayload(s.RawEvents, s.Events, richTraces)

	body := map[string]any{
		"tool":            s.Tool,
		"started_at":      s.StartTime.Format(time.RFC3339Nano),
		"ended_at":        s.EndTime.Format(time.RFC3339Nano),
		"duration_ms":     durationMs,
		"llm_call_count":  s.LLMCalls,
		"total_tokens":    s.TotalTokens,
		"estimated_cost":  s.EstimatedCost,
		"events":          events,
		"device_id":       deviceID,
		"conversation_id": s.ConversationID,
		"session_id":      sessionID,
		"generation_id":   s.GenerationID,
		"model":           s.Model,
	}

	if len(s.MCPToolUsage) > 0 {
		body["mcp_tool_usage"] = s.MCPToolUsage
	}
	if s.SessionEndReason != "" {
		body["session_end_reason"] = s.SessionEndReason
	}
	if s.SessionDurationMs > 0 {
		body["session_duration_ms"] = s.SessionDurationMs
	}
	if s.RepoName != "" {
		body["repo_name"] = s.RepoName
	}
	if s.RepoURLHash != "" {
		body["repo_url_hash"] = s.RepoURLHash
	}
	if s.BranchName != "" {
		body["branch_name"] = s.BranchName
	}
	if s.CommitSHA != "" {
		body["commit_sha"] = s.CommitSHA
	}
	if len(s.FilesModified) > 0 {
		sanitized := make([]map[string]any, len(s.FilesModified))
		for i, entry := range s.FilesModified {
			sanitizedEntry := make(map[string]any, len(entry))
			for k, v := range entry {
				if k == "file_path" {
					if fp, ok := v.(string); ok {
						sanitizedEntry[k] = SanitizePath(fp)
						continue
					}
				}
				sanitizedEntry[k] = v
			}
			sanitized[i] = sanitizedEntry
		}
		body["files_modified"] = sanitized
	}

	return body
}

// buildEventPayload converts raw events or structured events into API-ready maps.
// When richTraces is true, tool inputs/outputs and command content are included (truncated to 10KB).
func buildEventPayload(rawEvents []map[string]any, events []Event, richTraces bool) []map[string]any {
	if len(rawEvents) > 0 {
		return rawEvents
	}

	var result []map[string]any
	for _, ev := range events {
		evMap := map[string]any{
			"hook_type":       string(ev.HookType),
			"normalized_type": ev.NormalizedType,
			"timestamp":       ev.Timestamp.Format(time.RFC3339Nano),
			"tool_name":       ev.ToolName,
			"file_path":       ev.FilePath,
			"duration_ms":     ev.DurationMs,
			"conversation_id": ev.ConversationID,
			"session_id":      ev.SessionID,
			"tokens": map[string]int{
				"input":    ev.InputTokens,
				"output":   ev.OutputTokens,
				"thinking": ev.ThinkingTokens,
			},
		}
		if ev.CompactionTrigger != "" {
			evMap["compaction_trigger"] = ev.CompactionTrigger
		}
		if ev.ContextUsagePercent > 0 {
			evMap["context_usage_percent"] = ev.ContextUsagePercent
		}
		if ev.ContextTokens > 0 {
			evMap["context_tokens"] = ev.ContextTokens
		}
		if ev.ContextWindowSize > 0 {
			evMap["context_window_size"] = ev.ContextWindowSize
		}
		if ev.MessageCount > 0 {
			evMap["message_count"] = ev.MessageCount
		}
		if ev.MessagesToCompact > 0 {
			evMap["messages_to_compact"] = ev.MessagesToCompact
		}
		if ev.IsFirstCompaction != nil {
			evMap["is_first_compaction"] = *ev.IsFirstCompaction
		}
		if richTraces {
			if len(ev.ToolInput) > 0 {
				input := string(ev.ToolInput)
				if len(input) > 10000 {
					input = input[:10000]
				}
				evMap["tool_input"] = input
			}
			if len(ev.ToolOutput) > 0 {
				output := string(ev.ToolOutput)
				if len(output) > 10000 {
					output = output[:10000]
				}
				evMap["tool_output"] = output
			}
			if ev.Command != "" {
				cmd := ev.Command
				if len(cmd) > 10000 {
					cmd = cmd[:10000]
				}
				evMap["command"] = cmd
			}
			if ev.CommandOutput != "" {
				cmdOut := ev.CommandOutput
				if len(cmdOut) > 10000 {
					cmdOut = cmdOut[:10000]
				}
				evMap["command_output"] = cmdOut
			}
		}
		if ev.ParentSessionID != "" {
			evMap["parent_session_id"] = ev.ParentSessionID
		}
		if ev.SubagentName != "" {
			evMap["subagent_name"] = ev.SubagentName
		}
		if ev.SubagentRole != "" {
			evMap["subagent_role"] = ev.SubagentRole
		}
		if ev.SubagentDepth > 0 {
			evMap["subagent_depth"] = ev.SubagentDepth
		}
		if ev.TokenBreakdownData != nil {
			evMap["token_breakdown"] = ev.TokenBreakdownData
		}
		result = append(result, evMap)
	}
	return result
}

