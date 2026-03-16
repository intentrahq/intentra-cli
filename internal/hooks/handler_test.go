package hooks

import (
	"bytes"
	"testing"

	"github.com/intentrahq/intentra-cli/internal/config"
)

func TestProcessEvent_ParsesEvent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Enabled = true
	cfg.Server.Endpoint = "http://localhost:9999/v1"
	cfg.Server.Auth.Mode = config.AuthModeAPIKey
	cfg.Server.Auth.APIKey.KeyID = "test-key"
	cfg.Server.Auth.APIKey.Secret = "test-secret"

	promptInput := `{"conversation_id": "test-123"}`
	promptReader := bytes.NewBufferString(promptInput)
	err := ProcessEventWithEvent(promptReader, cfg, "cursor", "beforeSubmitPrompt")
	if err != nil {
		t.Errorf("Unexpected error buffering prompt event: %v", err)
	}

	stopInput := `{"conversation_id": "test-123"}`
	stopReader := bytes.NewBufferString(stopInput)
	err = ProcessEventWithEvent(stopReader, cfg, "cursor", "stop")
	if err != nil {
		t.Errorf("Unexpected error on stop event: %v", err)
	}
}

func TestProcessEvent_ToolInputDoesNotBreakMarshal(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Enabled = false

	toolUseInput := `{"session_id":"sess-456","tool_name":"Bash","tool_input":{"command":"ls -la"}}`
	reader := bytes.NewBufferString(toolUseInput)
	err := ProcessEventWithEvent(reader, cfg, "claude", "PostToolUse")
	if err != nil {
		t.Fatalf("PostToolUse with tool_input should not error, got: %v", err)
	}

	toolUseOutput := `{"session_id":"sess-456","tool_name":"Read","tool_input":{"file_path":"/tmp/test.go"},"tool_output":{"content":"package main"}}`
	reader2 := bytes.NewBufferString(toolUseOutput)
	err = ProcessEventWithEvent(reader2, cfg, "claude", "PostToolUse")
	if err != nil {
		t.Fatalf("PostToolUse with tool_input and tool_output should not error, got: %v", err)
	}

	preToolInput := `{"session_id":"sess-456","tool_name":"Write","tool_input":{"file_path":"/tmp/out.go","content":"package main\nfunc main() {}"}}`
	reader3 := bytes.NewBufferString(preToolInput)
	err = ProcessEventWithEvent(reader3, cfg, "claude", "PreToolUse")
	if err != nil {
		t.Fatalf("PreToolUse with tool_input should not error, got: %v", err)
	}
}

func TestRunHookHandlerWithTool_RequiresConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Enabled = false

	emptyInput := bytes.NewBufferString("")
	err := ProcessEventWithEvent(emptyInput, cfg, "cursor", "stop")
	if err != nil {
		t.Errorf("Empty input should not return error, got: %v", err)
	}

	cfg.Server.Enabled = true
	cfg.Server.Endpoint = ""
	validInput := bytes.NewBufferString(`{"conversation_id": "test"}`)
	err = ProcessEventWithEvent(validInput, cfg, "cursor", "stop")
	if err != nil {
		t.Errorf("Should not error with empty endpoint (fails silently), got: %v", err)
	}
}
