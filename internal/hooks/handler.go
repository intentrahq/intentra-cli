// Package hooks manages integration with AI coding tools by installing and
// handling event hooks. It supports Cursor, Claude Code, Gemini CLI, GitHub
// Copilot, and Windsurf Cascade, providing real-time event capture and
// forwarding to the Intentra API.
package hooks

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/intentrahq/intentra-cli/internal/api"
	"github.com/intentrahq/intentra-cli/internal/auth"
	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/debug"
	"github.com/intentrahq/intentra-cli/internal/device"
	"github.com/intentrahq/intentra-cli/internal/queue"
	"github.com/intentrahq/intentra-cli/internal/scanner"
	"github.com/intentrahq/intentra-cli/pkg/models"
)

const (
	maxBufferAge = 30 * time.Minute
)

const cleanupMarkerFile = "intentra_cleanup_marker"

type bufferedEvent struct {
	Event    *models.Event  `json:"event"`
	RawEvent map[string]any `json:"raw_event"`
}

func getBufferPath(sessionKey string) string {
	hash := sha256.Sum256([]byte(sessionKey))
	filename := "intentra_buffer_" + hex.EncodeToString(hash[:8]) + ".jsonl"
	return filepath.Join(os.TempDir(), filename)
}

func getLastScanPath(sessionKey string) string {
	hash := sha256.Sum256([]byte(sessionKey))
	filename := "intentra_lastscan_" + hex.EncodeToString(hash[:8]) + ".txt"
	return filepath.Join(os.TempDir(), filename)
}

func saveLastScanID(sessionKey, scanID string) {
	path := getLastScanPath(sessionKey)
	if err := os.WriteFile(path, []byte(scanID), 0600); err != nil {
		debug.Log("failed to write scan ID file: %v", err)
	}
}

func getLastScanID(sessionKey string) string {
	path := getLastScanPath(sessionKey)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func clearLastScanID(sessionKey string) {
	path := getLastScanPath(sessionKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		debug.Log("failed to remove scan ID file: %v", err)
	}
}

func appendToBuffer(sessionKey string, event *models.Event, rawEvent map[string]any) error {
	bufferPath := getBufferPath(sessionKey)
	f, err := os.OpenFile(bufferPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open buffer: %w", err)
	}
	defer f.Close()

	entry := bufferedEvent{Event: event, RawEvent: rawEvent}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write to buffer: %w", err)
	}

	return nil
}

func readAndClearBuffer(sessionKey string) ([]bufferedEvent, error) {
	bufferPath := getBufferPath(sessionKey)

	// Atomically move the buffer file to a temp name before reading.
	// This prevents concurrent writers from losing events between read and delete.
	tmpPath := bufferPath + ".reading"
	if err := os.Rename(bufferPath, tmpPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to move buffer for reading: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read buffer: %w", err)
	}

	os.Remove(tmpPath)

	var events []bufferedEvent
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry bufferedEvent
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		events = append(events, entry)
	}

	return events, nil
}

func cleanupStaleBuffers() {
	markerPath := filepath.Join(os.TempDir(), cleanupMarkerFile)
	if info, err := os.Stat(markerPath); err == nil {
		if time.Since(info.ModTime()) <= time.Hour {
			return
		}
	}

	// Touch/create the cleanup marker file
	if f, err := os.Create(markerPath); err == nil {
		f.Close()
	}

	patterns := []string{
		filepath.Join(os.TempDir(), "intentra_buffer_*.jsonl"),
		filepath.Join(os.TempDir(), "intentra_lastscan_*.txt"),
	}

	cutoff := time.Now().Add(-maxBufferAge)
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(f)
			}
		}
	}
}

func collectGitMetadata() (repoName, repoURLHash, branchName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output(); err == nil {
		remoteURL := strings.TrimSpace(string(out))
		if remoteURL != "" {
			hash := sha256.Sum256([]byte(remoteURL))
			repoURLHash = hex.EncodeToString(hash[:])

			name := remoteURL
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			if idx := strings.LastIndex(name, ":"); idx >= 0 {
				name = name[idx+1:]
			}
			name = strings.TrimSuffix(name, ".git")
			repoName = name
		}
	}

	if out, err := exec.CommandContext(ctx, "git", "branch", "--show-current").Output(); err == nil {
		branchName = strings.TrimSpace(string(out))
	}

	return
}

// --- createAggregatedScan and helpers ---

func createAggregatedScan(events []bufferedEvent, tool string) *models.Scan {
	if len(events) == 0 {
		return nil
	}

	scan := initScan(events, tool)
	aggregateEventMetrics(events, scan)

	scan.Model = detectFirstString(events, func(e *models.Event) string { return e.Model })
	scan.GenerationID = detectFirstString(events, func(e *models.Event) string { return e.GenerationID })

	model := scan.Model
	if model == "" {
		if tool == "copilot" {
			model = "gpt-4o"
		} else {
			model = "claude-sonnet-4.5"
		}
	}
	scan.EstimatedCost = scanner.EstimateCost(scan.TotalTokens, model, tool)

	scan.MCPToolUsage = aggregateMCPToolUsage(events, scan.EstimatedCost)

	repoName, repoURLHash, branchName := collectGitMetadata()
	scan.RepoName = repoName
	scan.RepoURLHash = repoURLHash
	scan.BranchName = branchName

	var allEvents []models.Event
	for _, entry := range events {
		allEvents = append(allEvents, *entry.Event)
	}
	scan.FilesModified = scanner.AggregateFilesModified(allEvents)

	extractSessionEndMetadata(scan, tool, events)

	return scan
}

func initScan(events []bufferedEvent, tool string) *models.Scan {
	first := events[0]
	last := events[len(events)-1]

	scan := &models.Scan{
		Tool:           tool,
		ConversationID: first.Event.ConversationID,
		Status:         models.ScanStatusPending,
		StartTime:      first.Event.Timestamp,
		EndTime:        last.Event.Timestamp,
		DeviceID:       first.Event.DeviceID,
	}

	if scan.ConversationID == "" && first.Event.SessionID != "" {
		scan.ConversationID = first.Event.SessionID
	}

	scan.ID = models.GenerateScanID(scan.ConversationID, scan.StartTime)

	scan.Source = &models.ScanSource{
		Tool:      tool,
		SessionID: first.Event.SessionID,
	}

	return scan
}

func aggregateEventMetrics(events []bufferedEvent, scan *models.Scan) {
	const maxPreCompactEvents = 10
	preCompactCount := 0

	for _, entry := range events {
		ev := entry.Event
		normalizedType := NormalizedEventType(ev.NormalizedType)

		if normalizedType == EventPreCompact {
			preCompactCount++
			if preCompactCount > maxPreCompactEvents {
				continue
			}
		}

		scan.Events = append(scan.Events, *ev)

		rawEvent := entry.RawEvent
		if rawEvent == nil {
			rawEvent = make(map[string]any)
		}
		rawEvent["normalized_type"] = ev.NormalizedType
		scan.RawEvents = append(scan.RawEvents, rawEvent)

		scan.InputTokens += ev.InputTokens
		scan.OutputTokens += ev.OutputTokens
		scan.ThinkingTokens += ev.ThinkingTokens

		if IsLLMCallEvent(normalizedType) {
			scan.LLMCalls++
		}
		if IsToolCallEvent(normalizedType) {
			scan.ToolCalls++
		}
	}

	scan.TotalTokens = scan.InputTokens + scan.OutputTokens + scan.ThinkingTokens
}

func detectFirstString(events []bufferedEvent, extract func(*models.Event) string) string {
	for _, entry := range events {
		if v := extract(entry.Event); v != "" {
			return v
		}
	}
	return ""
}

// extractInt64 extracts an int64 from a map value that may be float64 or json.Number.
func extractInt64(raw map[string]any, key string) int64 {
	switch v := raw[key].(type) {
	case float64:
		return int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
	}
	return 0
}

func extractSessionEndMetadata(scan *models.Scan, tool string, events []bufferedEvent) {
	if tool != "copilot" && tool != "gemini" {
		return
	}
	for i := len(events) - 1; i >= 0; i-- {
		entry := events[i]
		if NormalizedEventType(entry.Event.NormalizedType) == EventSessionEnd && entry.RawEvent != nil {
			if reason, ok := entry.RawEvent["reason"].(string); ok && reason != "" {
				scan.SessionEndReason = reason
			}
			scan.SessionDurationMs = extractInt64(entry.RawEvent, "duration_ms")
			break
		}
	}
}

// aggregateMCPToolUsage builds per-server/tool usage summaries from buffered events.
// Cost is attributed proportionally based on MCP call duration vs total scan duration.
func aggregateMCPToolUsage(events []bufferedEvent, totalScanCost float64) []models.MCPToolCall {
	type mcpKey struct {
		serverName string
		toolName   string
		urlHash    string
	}

	usage := make(map[mcpKey]*models.MCPToolCall)
	totalScanDuration := 0

	for _, entry := range events {
		ev := entry.Event
		totalScanDuration += ev.DurationMs

		if !ev.IsMCPEvent() {
			continue
		}

		urlHash := models.MCPServerURLHash(ev.MCPServerURL, ev.MCPServerCmd)
		key := mcpKey{
			serverName: ev.MCPServerName,
			toolName:   ev.MCPToolName,
			urlHash:    urlHash,
		}

		call, exists := usage[key]
		if !exists {
			call = &models.MCPToolCall{
				ServerName:    ev.MCPServerName,
				ToolName:      ev.MCPToolName,
				ServerURLHash: urlHash,
			}
			usage[key] = call
		}

		call.CallCount++
		call.TotalDuration += ev.DurationMs

		if ev.Error != "" {
			call.ErrorCount++
		}
	}

	if len(usage) == 0 {
		return nil
	}

	var result []models.MCPToolCall
	for _, call := range usage {
		if totalScanDuration > 0 && totalScanCost > 0 {
			proportion := float64(call.TotalDuration) / float64(totalScanDuration)
			// Cap MCP cost attribution at 20% of total scan cost
			// MCP calls are typically cheap API calls; LLM tokens drive real cost
			if proportion > 0.2 {
				proportion = 0.2
			}
			call.EstimatedCost = totalScanCost * proportion
		}
		result = append(result, *call)
	}

	return result
}

// --- normalizeHookEvent and helpers ---

func normalizeHookEvent(rawJSON []byte, tool, eventType string) (*models.Event, map[string]any, NormalizedEventType, error) {
	var raw map[string]any
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, nil, EventUnknown, fmt.Errorf("failed to parse raw JSON: %w", err)
	}

	normalizer := GetNormalizer(tool)
	normalizedType := normalizer.NormalizeEventType(eventType)

	event := &models.Event{
		Tool:           tool,
		HookType:       models.HookType(eventType),
		NormalizedType: string(normalizedType),
	}

	extractIdentifiers(event, raw)
	extractToolMetadata(event, raw)
	extractToolIO(event, raw)
	extractContentFields(event, raw)
	extractErrorFields(event, raw)
	extractMCPMetadata(event, raw, tool, normalizedType)
	extractCompactionMetadata(event, raw, normalizedType)

	sanitizeEvent(event)

	return event, raw, normalizedType, nil
}

func extractIdentifiers(event *models.Event, raw map[string]any) {
	if v, ok := raw["conversation_id"].(string); ok {
		event.ConversationID = v
	}
	if v, ok := raw["session_id"].(string); ok {
		event.SessionID = v
	}
	if v, ok := raw["trajectory_id"].(string); ok && event.ConversationID == "" {
		event.ConversationID = v
	}

	if v, ok := raw["generation_id"].(string); ok {
		event.GenerationID = v
	}
	if v, ok := raw["execution_id"].(string); ok && event.GenerationID == "" {
		event.GenerationID = v
	}
	if v, ok := raw["turn_id"].(string); ok && event.GenerationID == "" {
		event.GenerationID = v
	}
}

func extractToolMetadata(event *models.Event, raw map[string]any) {
	if v, ok := raw["hook_event_name"].(string); ok && event.HookType == "" {
		event.HookType = models.HookType(v)
	}
	if v, ok := raw["agent_action_name"].(string); ok && event.HookType == "" {
		event.HookType = models.HookType(v)
	}

	if v, ok := raw["model"].(string); ok {
		event.Model = v
	}
	if v, ok := raw["user_email"].(string); ok {
		event.UserEmail = v
	}

	if v, ok := raw["tool_name"].(string); ok {
		event.ToolName = v
	}
	if v, ok := raw["toolName"].(string); ok && event.ToolName == "" {
		event.ToolName = v
	}
}

func extractToolIO(event *models.Event, raw map[string]any) {
	if toolInput, ok := raw["tool_input"].(map[string]any); ok {
		if inputJSON, err := json.Marshal(toolInput); err == nil {
			event.ToolInput = inputJSON
		}
		if cmd, ok := toolInput["command"].(string); ok {
			event.Command = cmd
		}
		if fp, ok := toolInput["file_path"].(string); ok {
			event.FilePath = fp
		}
	}

	if toolArgs, ok := raw["toolArgs"].(string); ok && event.ToolInput == nil {
		if b, err := json.Marshal(toolArgs); err == nil {
			event.ToolInput = json.RawMessage(b)
		}
	}

	if toolOutput, ok := raw["tool_output"].(string); ok {
		if escaped, err := json.Marshal(toolOutput); err == nil {
			event.ToolOutput = json.RawMessage(escaped)
		}
	} else if toolOutput, ok := raw["tool_output"].(map[string]any); ok {
		if outputJSON, err := json.Marshal(toolOutput); err == nil {
			event.ToolOutput = outputJSON
		}
	}
	if toolResp, ok := raw["tool_response"].(map[string]any); ok {
		if respJSON, err := json.Marshal(toolResp); err == nil {
			event.ToolOutput = respJSON
		}
	}
	if toolResult, ok := raw["toolResult"].(map[string]any); ok {
		if resultJSON, err := json.Marshal(toolResult); err == nil {
			event.ToolOutput = resultJSON
		}
	}

	if toolInfo, ok := raw["tool_info"].(map[string]any); ok {
		if fp, ok := toolInfo["file_path"].(string); ok {
			event.FilePath = fp
		}
		if cmd, ok := toolInfo["command_line"].(string); ok {
			event.Command = cmd
		}
		if prompt, ok := toolInfo["user_prompt"].(string); ok {
			event.Prompt = prompt
		}
		if resp, ok := toolInfo["response"].(string); ok {
			event.Response = resp
		}
		if toolInfoJSON, err := json.Marshal(toolInfo); err == nil {
			if event.ToolInput == nil {
				event.ToolInput = toolInfoJSON
			}
		}
	}
}

func extractContentFields(event *models.Event, raw map[string]any) {
	if v, ok := raw["command"].(string); ok && event.Command == "" {
		event.Command = v
	}
	if v, ok := raw["output"].(string); ok {
		event.CommandOutput = v
	}

	if v, ok := raw["prompt"].(string); ok {
		event.Prompt = v
	}
	if v, ok := raw["initialPrompt"].(string); ok && event.Prompt == "" {
		event.Prompt = v
	}
	if v, ok := raw["response"].(string); ok {
		event.Response = v
	}
	if v, ok := raw["thought"].(string); ok {
		event.Thought = v
	}
	if v, ok := raw["text"].(string); ok {
		if event.Response == "" {
			event.Response = v
		}
	}

	if v, ok := raw["file_path"].(string); ok && event.FilePath == "" {
		event.FilePath = v
	}
	if v, ok := raw["cwd"].(string); ok && event.FilePath == "" {
		event.FilePath = v
	}

	if v, ok := raw["duration"].(float64); ok {
		event.DurationMs = int(v)
	}
	if v, ok := raw["duration_ms"].(float64); ok {
		event.DurationMs = int(v)
	}

	if v, ok := raw["input_tokens"].(float64); ok {
		event.InputTokens = int(v)
	}
	if v, ok := raw["output_tokens"].(float64); ok {
		event.OutputTokens = int(v)
	}
}

func extractErrorFields(event *models.Event, raw map[string]any) {
	if errObj, ok := raw["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok {
			event.Response = "Error: " + msg
			event.Error = msg
		}
	} else if errStr, ok := raw["error"].(string); ok && errStr != "" {
		event.Error = errStr
	}
}

// redactContent replaces a content string with a length-preserving placeholder.
// This prevents PII/PHI from being stored in local buffers or transmitted to the API.
func redactContent(value string) string {
	if value == "" {
		return value
	}
	return "[redacted: " + strconv.Itoa(len(value)) + " chars]"
}

// sanitizeEvent redacts sensitive content fields on an event in place.
// Token counts and metadata are preserved; only content strings are replaced.
func sanitizeEvent(event *models.Event) {
	event.Prompt = redactContent(event.Prompt)
	event.Response = redactContent(event.Response)
	event.Thought = redactContent(event.Thought)
	event.Command = redactContent(event.Command)
	event.CommandOutput = redactContent(event.CommandOutput)
	if len(event.ToolInput) > 0 {
		event.ToolInput = nil
	}
	if len(event.ToolOutput) > 0 {
		event.ToolOutput = nil
	}
	event.FilePath = models.SanitizePath(event.FilePath)
}

// extractCompactionMetadata populates compaction-specific fields for pre_compact events.
// Cursor provides rich context window metrics; Claude Code and Gemini CLI provide only trigger type.
func extractCompactionMetadata(event *models.Event, raw map[string]any, normalizedType NormalizedEventType) {
	if normalizedType != EventPreCompact {
		return
	}

	if v, ok := raw["trigger"].(string); ok {
		if v == "auto" || v == "manual" {
			event.CompactionTrigger = v
		}
	}

	if v, ok := raw["context_usage_percent"].(float64); ok {
		event.ContextUsagePercent = min(max(int(v), 0), 100)
	}

	if v, ok := raw["context_tokens"].(float64); ok && v >= 0 {
		event.ContextTokens = int(v)
	}

	if v, ok := raw["context_window_size"].(float64); ok && v >= 0 {
		event.ContextWindowSize = int(v)
	}

	if v, ok := raw["message_count"].(float64); ok && v >= 0 {
		event.MessageCount = int(v)
	}

	if v, ok := raw["messages_to_compact"].(float64); ok && v >= 0 {
		event.MessagesToCompact = int(v)
	}

	if v, ok := raw["is_first_compaction"].(bool); ok {
		event.IsFirstCompaction = &v
	}
}

// extractMCPMetadata populates MCP-specific fields on the event based on the tool type.
// Each AI coding tool exposes MCP data in a different format.
func extractMCPMetadata(event *models.Event, raw map[string]any, tool string, normalizedType NormalizedEventType) {
	isMCPHook := normalizedType == EventBeforeMCP || normalizedType == EventAfterMCP
	isMCPToolUse := strings.HasPrefix(event.ToolName, "MCP:") || strings.HasPrefix(event.ToolName, "mcp__")

	if !isMCPHook && !isMCPToolUse {
		return
	}

	if isMCPToolUse && !isMCPHook {
		toolName := event.ToolName
		if strings.HasPrefix(toolName, "MCP:") {
			fullToolName := toolName[4:]
			event.MCPToolName = fullToolName
			event.MCPServerName = inferMCPServerName(fullToolName)
		} else if serverName, mcpTool, ok := models.ParseMCPDoubleUnderscoreName(toolName); ok {
			event.MCPServerName = serverName
			event.MCPToolName = mcpTool
		}
		return
	}

	switch tool {
	case "cursor":
		extractCursorMCP(event, raw)
	case "windsurf":
		extractWindsurfMCP(event, raw)
	case "claude", "gemini":
		extractClaudeGeminiMCP(event, raw)
	case "copilot":
		extractCopilotMCP(event, raw)
	default:
		if event.ToolName != "" {
			event.MCPToolName = event.ToolName
		}
	}
}

// extractCursorMCP handles Cursor's beforeMCPExecution / afterMCPExecution format.
// Input contains tool_name directly, plus server url or command.
func extractCursorMCP(event *models.Event, raw map[string]any) {
	if v, ok := raw["tool_name"].(string); ok {
		event.MCPToolName = v
	}

	if v, ok := raw["url"].(string); ok {
		event.MCPServerURL = models.SanitizeMCPServerURL(v)
	}
	if v, ok := raw["command"].(string); ok && event.MCPServerURL == "" {
		event.MCPServerCmd = models.SanitizeMCPServerCmd(v)
	}

	if event.MCPServerName == "" {
		if event.MCPServerURL != "" {
			event.MCPServerName = extractHostFromURL(event.MCPServerURL)
		} else if event.MCPServerCmd != "" {
			event.MCPServerName = event.MCPServerCmd
		} else if event.MCPToolName != "" {
			event.MCPServerName = inferMCPServerName(event.MCPToolName)
		}
	}
}

// extractWindsurfMCP handles Windsurf's pre_mcp_tool_use / post_mcp_tool_use format.
// Data is nested inside tool_info with explicit mcp_server_name and mcp_tool_name.
func extractWindsurfMCP(event *models.Event, raw map[string]any) {
	toolInfo, ok := raw["tool_info"].(map[string]any)
	if !ok {
		return
	}

	if v, ok := toolInfo["mcp_server_name"].(string); ok {
		event.MCPServerName = v
	}
	if v, ok := toolInfo["mcp_tool_name"].(string); ok {
		event.MCPToolName = v
	}
}

// extractClaudeGeminiMCP handles Claude Code and Gemini CLI MCP tool format.
// Tool names follow the pattern mcp__<server>__<tool>.
func extractClaudeGeminiMCP(event *models.Event, raw map[string]any) {
	toolName := event.ToolName
	if toolName == "" {
		if v, ok := raw["tool_name"].(string); ok {
			toolName = v
		}
	}

	if serverName, mcpTool, ok := models.ParseMCPDoubleUnderscoreName(toolName); ok {
		event.MCPServerName = serverName
		event.MCPToolName = mcpTool
	}
}

// extractCopilotMCP handles GitHub Copilot MCP calls.
// Copilot does not expose server names, so we use a pseudo-server.
func extractCopilotMCP(event *models.Event, _ map[string]any) {
	event.MCPServerName = "copilot-mcp"
	if event.ToolName != "" {
		event.MCPToolName = event.ToolName
	}
}

var mcpToolToServer = map[string]string{
	"browser_click":            "cursor-browser",
	"browser_close":            "cursor-browser",
	"browser_console_messages": "cursor-browser",
	"browser_fill":             "cursor-browser",
	"browser_fill_form":        "cursor-browser",
	"browser_get_attribute":    "cursor-browser",
	"browser_get_bounding_box": "cursor-browser",
	"browser_get_input_value":  "cursor-browser",
	"browser_handle_dialog":    "cursor-browser",
	"browser_highlight":        "cursor-browser",
	"browser_hover":            "cursor-browser",
	"browser_is_checked":       "cursor-browser",
	"browser_is_enabled":       "cursor-browser",
	"browser_is_visible":       "cursor-browser",
	"browser_lock":             "cursor-browser",
	"browser_navigate":         "cursor-browser",
	"browser_navigate_back":    "cursor-browser",
	"browser_navigate_forward": "cursor-browser",
	"browser_network_requests": "cursor-browser",
	"browser_press_key":        "cursor-browser",
	"browser_reload":           "cursor-browser",
	"browser_resize":           "cursor-browser",
	"browser_run_code":         "cursor-browser",
	"browser_scroll":           "cursor-browser",
	"browser_search":           "cursor-browser",
	"browser_select_option":    "cursor-browser",
	"browser_snapshot":         "cursor-browser",
	"browser_tabs":             "cursor-browser",
	"browser_take_screenshot":  "cursor-browser",
	"browser_type":             "cursor-browser",
	"browser_unlock":           "cursor-browser",
	"browser_wait_for":         "cursor-browser",

	"navigate_page":   "chrome-devtools",
	"evaluate_script": "chrome-devtools",
	"list_pages":      "chrome-devtools",
	"new_page":        "chrome-devtools",
	"take_snapshot":   "chrome-devtools",
	"take_screenshot": "chrome-devtools",
	"navigate":        "chrome-devtools",
	"select_page":     "chrome-devtools",
	"close_page":      "chrome-devtools",
	"click":           "chrome-devtools",
	"hover":           "chrome-devtools",
	"fill":            "chrome-devtools",
	"fill_form":       "chrome-devtools",
	"drag":            "chrome-devtools",
	"press_key":       "chrome-devtools",
	"upload_file":     "chrome-devtools",
	"wait_for":        "chrome-devtools",
	"handle_dialog":   "chrome-devtools",
	"emulate":         "chrome-devtools",
	"resize_page":     "chrome-devtools",

	"search_issues":      "sentry",
	"get_issue_details":  "sentry",
	"find_organizations": "sentry",
	"find_projects":      "sentry",
	"find_releases":      "sentry",
	"search_events":      "sentry",
	"update_issue":       "sentry",

	"event-definitions-list": "posthog",
	"organizations-get":      "posthog",
	"projects-get":           "posthog",
	"list_teams":             "posthog",
	"list_projects":          "posthog",
	"list-errors":            "posthog",
}

func inferMCPServerName(toolName string) string {
	if server, ok := mcpToolToServer[toolName]; ok {
		return server
	}

	if strings.HasPrefix(toolName, "browser_") {
		return "cursor-browser"
	}

	if strings.Contains(toolName, "__") {
		parts := strings.SplitN(toolName, "__", 2)
		if len(parts) == 2 {
			return parts[0]
		}
	}

	return "mcp"
}

func extractHostFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return host
}

// --- ProcessEventWithEvent and helpers ---

// ProcessEventWithEvent buffers events and sends aggregated scan on stop events.
func ProcessEventWithEvent(reader io.Reader, cfg *config.Config, tool, eventType string) error {
	bufScanner := bufio.NewScanner(reader)
	bufScanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	if !bufScanner.Scan() {
		if err := bufScanner.Err(); err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		return nil
	}

	rawJSON := bufScanner.Bytes()
	if len(rawJSON) == 0 {
		return nil
	}

	event, rawMap, normalizedType, err := normalizeHookEvent(rawJSON, tool, eventType)
	if err != nil {
		return fmt.Errorf("failed to normalize event: %w", err)
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	if event.DeviceID == "" {
		deviceID, err := device.GetDeviceID()
		if err == nil {
			event.DeviceID = deviceID
		}
	}

	sessionKey, tool := deriveSessionKey(event, tool)

	if IsStopEvent(normalizedType, tool) {
		return handleStopEvent(sessionKey, tool, event, rawMap, cfg)
	}

	if IsSessionEndEvent(normalizedType, tool) {
		return handleSessionEndEvent(sessionKey, rawMap)
	}

	if err := appendToBuffer(sessionKey, event, rawMap); err != nil {
		return fmt.Errorf("failed to buffer event: %w", err)
	}

	return nil
}

func deriveSessionKey(event *models.Event, tool string) (string, string) {
	baseKey := event.ConversationID
	if baseKey == "" {
		baseKey = event.SessionID
	}
	if baseKey == "" {
		baseKey = event.DeviceID + "_default"
	}
	sessionKey := tool + "_" + baseKey

	if tool == "claude" {
		cursorKey := "cursor_" + baseKey
		cursorBufferPath := getBufferPath(cursorKey)
		if info, err := os.Stat(cursorBufferPath); err == nil {
			if time.Since(info.ModTime()) < 30*time.Minute {
				debug.Log("Claude event has matching active Cursor session, treating as Cursor")
				sessionKey = cursorKey
				tool = "cursor"
			} else {
				debug.Log("Claude event has matching but stale Cursor session, keeping separate")
			}
		}
	}

	return sessionKey, tool
}

func handleStopEvent(sessionKey, tool string, event *models.Event, rawMap map[string]any, cfg *config.Config) error {
	cleanupStaleBuffers()

	if err := appendToBuffer(sessionKey, event, rawMap); err != nil {
		return fmt.Errorf("failed to buffer event: %w", err)
	}

	bufferedEvents, err := readAndClearBuffer(sessionKey)
	if err != nil {
		return fmt.Errorf("failed to read buffer: %w", err)
	}

	if len(bufferedEvents) == 0 {
		return nil
	}

	scan := createAggregatedScan(bufferedEvents, tool)
	if scan == nil {
		return nil
	}

	synced := false

	creds, credErr := auth.GetValidCredentials()
	if credErr != nil {
		debug.Warn("credential check failed: %v", credErr)
	}
	if creds != nil {
		if err := api.SendScanWithJWT(scan, creds.AccessToken); err != nil {
			debug.Warn("failed to sync to api.intentra.sh: %v", err)
		} else {
			debug.Log("Synced to https://api.intentra.sh")
			synced = true
		}
	}

	if !synced && cfg.Server.Enabled {
		client, err := api.NewClient(cfg)
		if err == nil {
			debug.Log("Syncing to %s (config auth)", cfg.Server.Endpoint)
			if err := client.SendScan(scan); err != nil {
				debug.Warn("sync failed: %v", err)
			} else {
				synced = true
			}
		}
	}

	if !synced {
		if err := queue.Enqueue(scan); err != nil {
			debug.Warn("failed to queue scan offline: %v", err)
		}
	}

	if synced && scan.ID != "" {
		saveLastScanID(sessionKey, scan.ID)
		// Opportunistically flush any previously queued scans in background
		// to avoid blocking the hook handler hot path
		if creds != nil {
			go queue.FlushWithJWT(creds.AccessToken)
		}
	}

	if debug.Enabled {
		if err := scanner.SaveScan(scan); err != nil {
			debug.Warn("failed to save scan locally: %v", err)
		} else {
			debug.Log("Saved scan locally: %s", scan.ID)
		}
	}

	return nil
}

func handleSessionEndEvent(sessionKey string, rawMap map[string]any) error {
	lastScanID := getLastScanID(sessionKey)
	if lastScanID == "" {
		debug.Log("sessionEnd event but no lastScanID for session %s, ignoring", sessionKey)
		return nil
	}

	creds, err := auth.GetValidCredentials()
	if err != nil {
		debug.Warn("credential check failed: %v", err)
	}
	if creds == nil {
		debug.Log("sessionEnd event but no valid credentials, ignoring")
		return nil
	}

	reason := ""
	durationMs := int64(0)
	if rawMap != nil {
		if r, ok := rawMap["reason"].(string); ok {
			reason = r
		}
		durationMs = extractInt64(rawMap, "duration_ms")
	}

	if err := api.PatchSessionEnd(lastScanID, creds.AccessToken, reason, durationMs); err != nil {
		debug.Warn("failed to PATCH session end: %v", err)
	} else {
		debug.Log("PATCHed session end for scan %s", lastScanID)
	}

	clearLastScanID(sessionKey)
	return nil
}

// RunHookHandlerWithToolAndEvent processes hooks with tool and event identifiers.
func RunHookHandlerWithToolAndEvent(tool, event string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	debug.Enabled = cfg.Debug

	return ProcessEventWithEvent(os.Stdin, cfg, tool, event)
}

