// Package models provides data structures and types used throughout Intentra.
// It defines events, scans, and their associated metadata for tracking AI
// coding tool activity.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// TokenBreakdown provides per-category token attribution for context analysis.
type TokenBreakdown struct {
	SystemTokens   int `json:"system_tokens,omitempty"`
	UserTokens     int `json:"user_tokens,omitempty"`
	ToolTokens     int `json:"tool_tokens,omitempty"`
	ThinkingTokens int `json:"thinking_tokens,omitempty"`
	SubagentTokens int `json:"subagent_tokens,omitempty"`
	ContextFiles   int `json:"context_files_tokens,omitempty"`
}

// HookType represents the native hook event type from AI coding tools.
// This is the raw event type as received from the tool (camelCase, PascalCase, or snake_case).
type HookType string

// Event represents a single hook event from AI coding tools.
type Event struct {
	HookType       HookType  `json:"hook_type"`
	NormalizedType string    `json:"normalized_type"`
	Timestamp      time.Time `json:"timestamp"`
	ScanID         string    `json:"scan_id,omitempty"`
	ConversationID string    `json:"conversation_id"`
	SessionID      string    `json:"session_id,omitempty"`
	GenerationID   string    `json:"generation_id,omitempty"`
	Model          string    `json:"model,omitempty"`
	UserEmail      string    `json:"user_email,omitempty"`
	DeviceID       string    `json:"device_id,omitempty"`
	Tool           string    `json:"tool,omitempty"`

	Prompt        string          `json:"prompt,omitempty"`
	Response      string          `json:"response,omitempty"`
	Thought       string          `json:"thought,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolOutput    json.RawMessage `json:"tool_output,omitempty"`
	FilePath      string          `json:"file_path,omitempty"`
	Command       string          `json:"command,omitempty"`
	CommandOutput string          `json:"command_output,omitempty"`

	MCPServerName string `json:"mcp_server_name,omitempty"`
	MCPToolName   string `json:"mcp_tool_name,omitempty"`
	MCPServerURL  string `json:"mcp_server_url,omitempty"`
	MCPServerCmd  string `json:"mcp_server_cmd,omitempty"`

	InputTokens    int `json:"input_tokens,omitempty"`
	OutputTokens   int `json:"output_tokens,omitempty"`
	ThinkingTokens int `json:"thinking_tokens,omitempty"`
	DurationMs     int `json:"duration_ms,omitempty"`

	ContextUsagePercent int    `json:"context_usage_percent,omitempty"`
	ContextTokens       int    `json:"context_tokens,omitempty"`
	ContextWindowSize   int    `json:"context_window_size,omitempty"`
	MessageCount        int    `json:"message_count,omitempty"`
	MessagesToCompact   int    `json:"messages_to_compact,omitempty"`
	IsFirstCompaction   *bool  `json:"is_first_compaction,omitempty"`
	CompactionTrigger   string `json:"compaction_trigger,omitempty"`

	ParentSessionID    string          `json:"parent_session_id,omitempty"`
	SubagentName       string          `json:"subagent_name,omitempty"`
	SubagentRole       string          `json:"subagent_role,omitempty"`
	SubagentDepth      int             `json:"subagent_depth,omitempty"`
	TokenBreakdownData *TokenBreakdown `json:"token_breakdown,omitempty"`

	Error string `json:"error,omitempty"`
}

// IsMCPEvent returns true if this event is an MCP tool invocation.
func (e *Event) IsMCPEvent() bool {
	return e.MCPServerName != "" || e.MCPToolName != ""
}

// SanitizeMCPServerURL strips query parameters from a URL to prevent leaking API keys.
// Returns only scheme + host + path.
func SanitizeMCPServerURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// SanitizeMCPServerCmd extracts only the binary name from a command path.
// Prevents leaking local directory structures.
func SanitizeMCPServerCmd(cmd string) string {
	if cmd == "" {
		return ""
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	return filepath.Base(parts[0])
}

// ParseMCPDoubleUnderscoreName parses Claude Code and Gemini CLI MCP tool names
// in the format mcp__<server>__<tool>. Splits on the first two __ delimiters only.
func ParseMCPDoubleUnderscoreName(toolName string) (serverName, mcpToolName string, ok bool) {
	if !strings.HasPrefix(toolName, "mcp__") {
		return "", "", false
	}
	rest := toolName[5:]
	serverName, mcpToolName, found := strings.Cut(rest, "__")
	if !found {
		return rest, "", true
	}
	return serverName, mcpToolName, true
}

// NormalizedEventType represents a unified event type across all AI coding tools.
type NormalizedEventType string

const (
	EventSessionStart NormalizedEventType = "session_start"
	EventSessionEnd   NormalizedEventType = "session_end"

	EventBeforePrompt  NormalizedEventType = "before_prompt"
	EventAfterResponse NormalizedEventType = "after_response"
	EventAgentThought  NormalizedEventType = "agent_thought"

	EventBeforeTool NormalizedEventType = "before_tool"
	EventAfterTool  NormalizedEventType = "after_tool"

	EventBeforeFileRead NormalizedEventType = "before_file_read"
	EventAfterFileRead  NormalizedEventType = "after_file_read"
	EventBeforeFileEdit NormalizedEventType = "before_file_edit"
	EventAfterFileEdit  NormalizedEventType = "after_file_edit"

	EventBeforeShell NormalizedEventType = "before_shell"
	EventAfterShell  NormalizedEventType = "after_shell"

	EventBeforeMCP NormalizedEventType = "before_mcp"
	EventAfterMCP  NormalizedEventType = "after_mcp"

	EventBeforeModel NormalizedEventType = "before_model"
	EventAfterModel  NormalizedEventType = "after_model"

	EventToolSelection     NormalizedEventType = "tool_selection"
	EventPermissionRequest NormalizedEventType = "permission_request"
	EventNotification      NormalizedEventType = "notification"
	EventStop              NormalizedEventType = "stop"
	EventSubagentStart     NormalizedEventType = "subagent_start"
	EventSubagentStop      NormalizedEventType = "subagent_stop"
	EventPreCompact        NormalizedEventType = "pre_compact"
	EventError             NormalizedEventType = "error"
	EventToolUseFailure    NormalizedEventType = "tool_use_failure"
	EventWorktreeSetup     NormalizedEventType = "worktree_setup"
	EventUnknown           NormalizedEventType = "unknown"

	EventPostCompact            NormalizedEventType = "post_compact"
	EventTeammateIdle           NormalizedEventType = "teammate_idle"
	EventTaskCompleted          NormalizedEventType = "task_completed"
	EventInstructionsLoaded     NormalizedEventType = "instructions_loaded"
	EventConfigChange           NormalizedEventType = "config_change"
	EventWorktreeCreate         NormalizedEventType = "worktree_create"
	EventWorktreeRemove         NormalizedEventType = "worktree_remove"
	EventElicitation            NormalizedEventType = "elicitation"
	EventElicitationResult      NormalizedEventType = "elicitation_result"
	EventBeforeToolSelection    NormalizedEventType = "before_tool_selection"
	EventPreCompress            NormalizedEventType = "pre_compress"
	EventResponseWithTranscript NormalizedEventType = "response_with_transcript"
)

// IsLLMCallEvent returns true if the event represents an LLM call.
func IsLLMCallEvent(eventType NormalizedEventType) bool {
	return eventType == EventAfterResponse ||
		eventType == EventAfterTool ||
		eventType == EventAfterFileEdit ||
		eventType == EventAfterFileRead ||
		eventType == EventAfterShell ||
		eventType == EventAfterMCP ||
		eventType == EventAfterModel ||
		eventType == EventAgentThought
}

// IsToolCallEvent returns true if the event represents a tool execution.
func IsToolCallEvent(eventType NormalizedEventType) bool {
	return eventType == EventAfterTool ||
		eventType == EventAfterFileEdit ||
		eventType == EventAfterFileRead ||
		eventType == EventAfterShell ||
		eventType == EventAfterMCP
}

// MCPServerURLHash returns a short hash of the sanitized server URL or command.
// Used as a deduplication key alongside server name.
func MCPServerURLHash(serverURL, serverCmd string) string {
	input := serverURL + "|" + serverCmd
	if input == "|" {
		return ""
	}
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[:8]
}
