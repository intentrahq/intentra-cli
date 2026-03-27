package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventUnmarshal(t *testing.T) {
	jsonData := `{
		"hook_type": "afterAgentResponse",
		"normalized_type": "after_response",
		"timestamp": "2025-01-07T10:30:00Z",
		"conversation_id": "conv-123",
		"model": "claude-3-5-sonnet"
	}`

	var event Event
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if event.HookType != "afterAgentResponse" {
		t.Errorf("Expected afterAgentResponse, got %s", event.HookType)
	}
	if event.NormalizedType != "after_response" {
		t.Errorf("Expected after_response normalized type, got %s", event.NormalizedType)
	}
}

func TestEventUnmarshal_WithNewFields(t *testing.T) {
	jsonData := `{
		"hook_type": "afterTool",
		"normalized_type": "after_tool",
		"timestamp": "2025-01-07T10:30:00Z",
		"conversation_id": "conv-123",
		"generation_id": "gen-456",
		"error": "tool execution failed"
	}`

	var event Event
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if event.GenerationID != "gen-456" {
		t.Errorf("Expected gen-456, got %s", event.GenerationID)
	}
	if event.Error != "tool execution failed" {
		t.Errorf("Expected error field, got %s", event.Error)
	}
}

func TestScanMarshal(t *testing.T) {
	scan := Scan{
		ID:           "scan-123",
		Tool:         "cursor",
		Fingerprint:  "abc123",
		FilesHash:    "def456",
		ActionCounts: map[string]int{"edits": 5, "reads": 10},
	}

	data, err := json.Marshal(scan)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result Scan
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Fingerprint != "abc123" {
		t.Errorf("Expected fingerprint abc123, got %s", result.Fingerprint)
	}
	if result.FilesHash != "def456" {
		t.Errorf("Expected files_hash def456, got %s", result.FilesHash)
	}
	if result.ActionCounts["edits"] != 5 {
		t.Errorf("Expected edits count 5, got %d", result.ActionCounts["edits"])
	}
}

func TestBuildAPIPayload(t *testing.T) {
	t.Run("basic fields", func(t *testing.T) {
		scan := &Scan{
			Tool:           "cursor",
			ConversationID: "conv-123",
			Model:          "claude-sonnet-4.5",
			TotalTokens:    1000,
			LLMCalls:       5,
			EstimatedCost:  0.05,
			Source:         &ScanSource{SessionID: "sess-1"},
		}
		scan.StartTime, _ = time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
		scan.EndTime, _ = time.Parse(time.RFC3339, "2025-01-01T00:05:00Z")

		payload := scan.BuildAPIPayload("device-abc", false)

		if payload["tool"] != "cursor" {
			t.Errorf("tool = %v, want cursor", payload["tool"])
		}
		if payload["device_id"] != "device-abc" {
			t.Errorf("device_id = %v, want device-abc", payload["device_id"])
		}
		if payload["conversation_id"] != "conv-123" {
			t.Errorf("conversation_id = %v, want conv-123", payload["conversation_id"])
		}
		if payload["session_id"] != "sess-1" {
			t.Errorf("session_id = %v, want sess-1", payload["session_id"])
		}
		if payload["model"] != "claude-sonnet-4.5" {
			t.Errorf("model = %v, want claude-sonnet-4.5", payload["model"])
		}
		if payload["total_tokens"] != 1000 {
			t.Errorf("total_tokens = %v, want 1000", payload["total_tokens"])
		}
		if payload["llm_call_count"] != 5 {
			t.Errorf("llm_call_count = %v, want 5", payload["llm_call_count"])
		}
		if payload["duration_ms"] != int64(300000) {
			t.Errorf("duration_ms = %v, want 300000", payload["duration_ms"])
		}
	})

	t.Run("optional fields omitted when empty", func(t *testing.T) {
		scan := &Scan{Tool: "claude"}
		payload := scan.BuildAPIPayload("dev-1", false)

		if _, ok := payload["mcp_tool_usage"]; ok {
			t.Error("mcp_tool_usage should be omitted when empty")
		}
		if _, ok := payload["session_end_reason"]; ok {
			t.Error("session_end_reason should be omitted when empty")
		}
		if _, ok := payload["repo_name"]; ok {
			t.Error("repo_name should be omitted when empty")
		}
		if _, ok := payload["files_modified"]; ok {
			t.Error("files_modified should be omitted when empty")
		}
	})

	t.Run("optional fields included when set", func(t *testing.T) {
		scan := &Scan{
			Tool:             "claude",
			MCPToolUsage:     []MCPToolCall{{ServerName: "s", ToolName: "t", CallCount: 1}},
			SessionEndReason: "user_exit",
			RepoName:         "myrepo",
			RepoURLHash:      "abc123",
			BranchName:       "main",
		}
		payload := scan.BuildAPIPayload("dev-1", false)

		if payload["session_end_reason"] != "user_exit" {
			t.Errorf("session_end_reason = %v", payload["session_end_reason"])
		}
		if payload["repo_name"] != "myrepo" {
			t.Errorf("repo_name = %v", payload["repo_name"])
		}
		if payload["repo_url_hash"] != "abc123" {
			t.Errorf("repo_url_hash = %v", payload["repo_url_hash"])
		}
		if payload["branch_name"] != "main" {
			t.Errorf("branch_name = %v", payload["branch_name"])
		}
		mcpUsage, ok := payload["mcp_tool_usage"].([]MCPToolCall)
		if !ok || len(mcpUsage) != 1 {
			t.Errorf("mcp_tool_usage should have 1 entry")
		}
	})

	t.Run("nil source gives empty session_id", func(t *testing.T) {
		scan := &Scan{Tool: "cursor"}
		payload := scan.BuildAPIPayload("dev-1", false)
		if payload["session_id"] != "" {
			t.Errorf("session_id should be empty, got %v", payload["session_id"])
		}
	})

	t.Run("timestamp format", func(t *testing.T) {
		scan := &Scan{Tool: "cursor"}
		scan.StartTime, _ = time.Parse(time.RFC3339Nano, "2025-06-15T10:30:00.123456789Z")
		payload := scan.BuildAPIPayload("dev-1", false)
		startedAt, ok := payload["started_at"].(string)
		if !ok {
			t.Fatal("started_at not a string")
		}
		if _, err := time.Parse(time.RFC3339Nano, startedAt); err != nil {
			t.Errorf("started_at not valid RFC3339Nano: %s", startedAt)
		}
	})
}

func TestSanitizeMCPServerURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no query params", "https://example.com/api", "https://example.com/api"},
		{"strips query params", "https://example.com/api?key=secret&foo=bar", "https://example.com/api"},
		{"strips fragment", "https://example.com/api#section", "https://example.com/api"},
		{"invalid URL returned as-is", "://not-a-url", "://not-a-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeMCPServerURL(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeMCPServerURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeMCPServerCmd(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"basename only", "node", "node"},
		{"full path", "/usr/local/bin/node", "node"},
		{"with args", "/usr/bin/python3 -m server", "python3"},
		// Windows paths only work correctly on Windows; skip in cross-platform tests
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeMCPServerCmd(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeMCPServerCmd(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseMCPDoubleUnderscoreName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantOK     bool
	}{
		{"valid full name", "mcp__slack__post_message", "slack", "post_message", true},
		{"server only", "mcp__slack", "slack", "", true},
		{"not mcp prefix", "regular_tool_name", "", "", false},
		{"nested underscores in tool", "mcp__github__create_pull_request", "github", "create_pull_request", true},
		{"empty after mcp__", "mcp__", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, tool, ok := ParseMCPDoubleUnderscoreName(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if server != tt.wantServer {
				t.Errorf("server = %q, want %q", server, tt.wantServer)
			}
			if tool != tt.wantTool {
				t.Errorf("tool = %q, want %q", tool, tt.wantTool)
			}
		})
	}
}

func TestMCPServerURLHash(t *testing.T) {
	t.Run("empty inputs", func(t *testing.T) {
		if h := MCPServerURLHash("", ""); h != "" {
			t.Errorf("expected empty, got %q", h)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		h1 := MCPServerURLHash("https://example.com", "node")
		h2 := MCPServerURLHash("https://example.com", "node")
		if h1 != h2 {
			t.Errorf("not deterministic: %q != %q", h1, h2)
		}
		if len(h1) != 8 {
			t.Errorf("expected length 8, got %d", len(h1))
		}
	})

	t.Run("different inputs different hashes", func(t *testing.T) {
		h1 := MCPServerURLHash("https://a.com", "")
		h2 := MCPServerURLHash("https://b.com", "")
		if h1 == h2 {
			t.Error("different URLs should produce different hashes")
		}
	})
}

func TestIsMCPEvent(t *testing.T) {
	t.Run("server name set", func(t *testing.T) {
		e := &Event{MCPServerName: "slack"}
		if !e.IsMCPEvent() {
			t.Error("expected true")
		}
	})

	t.Run("tool name set", func(t *testing.T) {
		e := &Event{MCPToolName: "post_message"}
		if !e.IsMCPEvent() {
			t.Error("expected true")
		}
	})

	t.Run("neither set", func(t *testing.T) {
		e := &Event{}
		if e.IsMCPEvent() {
			t.Error("expected false")
		}
	})
}
