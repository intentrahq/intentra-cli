package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// ErrInvalidHandlerPath is returned when the handler path contains unsafe characters.
var ErrInvalidHandlerPath = errors.New("invalid handler path: contains unsafe characters")

// safePathPattern validates handler paths to prevent command injection.
// Allows alphanumeric, underscores, hyphens, dots, forward/back slashes, and colons (for Windows drives).
var safePathPattern = regexp.MustCompile(`^[a-zA-Z0-9/_\-\.:\\]+$`)

// validateHandlerPath checks if a handler path is safe to use in shell commands.
func validateHandlerPath(path string) error {
	if path == "" {
		return ErrInvalidHandlerPath
	}
	if len(path) > 4096 {
		return errors.New("invalid handler path: exceeds maximum length")
	}
	if !safePathPattern.MatchString(path) {
		return ErrInvalidHandlerPath
	}
	// Block common shell metacharacters and injection patterns
	dangerous := []string{";", "&", "|", "$", "`", "(", ")", "{", "}", "<", ">", "!", "~", "*", "?", "[", "]", "#", "\n", "\r", "'", "\""}
	for _, d := range dangerous {
		if strings.Contains(path, d) {
			return ErrInvalidHandlerPath
		}
	}
	return nil
}

// quotePathForShell safely quotes a path for shell execution.
// This provides defense-in-depth even though validateHandlerPath should catch issues.
func quotePathForShell(path string) string {
	if runtime.GOOS == "windows" {
		// Windows: wrap in double quotes and escape internal double quotes
		escaped := strings.ReplaceAll(path, "\"", "\\\"")
		return "\"" + escaped + "\""
	}
	// Unix: wrap in single quotes and escape internal single quotes
	escaped := strings.ReplaceAll(path, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// CursorHookConfig represents Cursor's hooks.json structure.
type CursorHookConfig struct {
	Version int                          `json:"version"`
	Hooks   map[string][]CursorHookEntry `json:"hooks"`
}

type CursorHookEntry struct {
	Command string `json:"command"`
}

// cursorHookTypes contains all available hooks per https://docs.cursor.com/context/hooks.
var cursorHookTypes = []string{
	"sessionStart",
	"sessionEnd",
	"beforeSubmitPrompt",
	"preToolUse",
	"postToolUse",
	"postToolUseFailure",
	"subagentStart",
	"subagentStop",
	"beforeShellExecution",
	"afterShellExecution",
	"beforeMCPExecution",
	"afterMCPExecution",
	"afterAgentResponse",
	"afterAgentThought",
	"afterFileEdit",
	"preCompact",
	"stop",
	"beforeTabFileRead",
	"beforeReadFile",
	"afterTabFileEdit",
}

// GenerateCursorHooksJSON creates the Cursor hooks.json content.
// Returns an error if the handler path contains unsafe characters.
func GenerateCursorHooksJSON(handlerPath string) (string, error) {
	// Validate handler path to prevent command injection
	if err := validateHandlerPath(handlerPath); err != nil {
		return "", err
	}

	config := CursorHookConfig{
		Version: 1,
		Hooks:   make(map[string][]CursorHookEntry),
	}

	for _, hookType := range cursorHookTypes {
		cmd := handlerPath
		if runtime.GOOS == "windows" {
			cmd = handlerPath + ".exe"
		}
		// Quote the path for safe shell execution
		quotedCmd := quotePathForShell(cmd)
		// Include event type in command for proper categorization
		config.Hooks[hookType] = []CursorHookEntry{{
			Command: quotedCmd + " hook --tool cursor --event " + hookType,
		}}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Cursor hooks JSON: %w", err)
	}
	return string(data), nil
}

// claudeCodeHookTypes contains all available hooks per https://code.claude.com/docs/en/hooks.
var claudeCodeHookTypes = []string{
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"PermissionRequest",
	"SessionStart",
	"SessionEnd",
	"Stop",
	"UserPromptSubmit",
	"Notification",
	"SubagentStart",
	"SubagentStop",
	"PreCompact",
	"PostCompact",
	"TeammateIdle",
	"TaskCompleted",
	"InstructionsLoaded",
	"ConfigChange",
	"WorktreeCreate",
	"WorktreeRemove",
	"Elicitation",
	"ElicitationResult",
}

// copilotHookTypes contains all available hooks per https://docs.github.com/en/copilot/concepts/agents/coding-agent/about-hooks.
var copilotHookTypes = []string{
	"sessionStart",
	"sessionEnd",
	"userPromptSubmitted",
	"preToolUse",
	"postToolUse",
	"agentStop",
	"subagentStop",
	"errorOccurred",
}

// windsurfHookTypes contains all available hooks per https://docs.windsurf.com/windsurf/cascade/hooks.
var windsurfHookTypes = []string{
	"pre_read_code",
	"post_read_code",
	"pre_write_code",
	"post_write_code",
	"pre_run_command",
	"post_run_command",
	"pre_mcp_tool_use",
	"post_mcp_tool_use",
	"pre_user_prompt",
	"post_cascade_response",
	"post_cascade_response_with_transcript",
	"post_setup_worktree",
}

// GenerateClaudeCodeHooks creates the Claude Code hooks configuration.
// Returns an error if the handler path contains unsafe characters.
func GenerateClaudeCodeHooks(handlerPath string) (map[string]any, error) {
	// Validate handler path to prevent command injection
	if err := validateHandlerPath(handlerPath); err != nil {
		return nil, err
	}

	cmd := handlerPath
	if runtime.GOOS == "windows" {
		cmd = handlerPath + ".exe"
	}

	// Quote the path for safe shell execution
	quotedCmd := quotePathForShell(cmd)

	// Claude Code uses a different hook structure
	// Hooks are defined per event type with matchers
	// matcher: ".*" matches all tools/events
	hooks := make(map[string]any)

	for _, hookType := range claudeCodeHookTypes {
		hooks[hookType] = []map[string]any{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{
						"type":    "command",
						"command": quotedCmd + " hook --tool claude --event " + hookType,
					},
				},
			},
		}
	}

	return hooks, nil
}

// CopilotHookConfig represents GitHub Copilot's hooks.json structure.
type CopilotHookConfig struct {
	Version int                          `json:"version"`
	Hooks   map[string][]CopilotHookItem `json:"hooks"`
}

type CopilotHookItem struct {
	Type       string `json:"type"`
	Bash       string `json:"bash,omitempty"`
	Powershell string `json:"powershell,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

// GenerateCopilotHooksJSON creates the GitHub Copilot hooks.json content.
func GenerateCopilotHooksJSON(handlerPath string) (string, error) {
	if err := validateHandlerPath(handlerPath); err != nil {
		return "", err
	}

	config := CopilotHookConfig{
		Version: 1,
		Hooks:   make(map[string][]CopilotHookItem),
	}

	quotedPath := quotePathForShell(handlerPath)
	windowsPath := handlerPath + ".exe"

	for _, hookType := range copilotHookTypes {
		config.Hooks[hookType] = []CopilotHookItem{{
			Type:       "command",
			Bash:       quotedPath + " hook --tool copilot --event " + hookType,
			Powershell: windowsPath + " hook --tool copilot --event " + hookType,
			TimeoutSec: 30,
		}}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Copilot hooks JSON: %w", err)
	}
	return string(data), nil
}

// WindsurfHookConfig represents Windsurf Cascade's hooks.json structure.
type WindsurfHookConfig struct {
	Hooks map[string][]WindsurfHookItem `json:"hooks"`
}

type WindsurfHookItem struct {
	Command          string `json:"command"`
	ShowOutput       bool   `json:"show_output,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

// GenerateWindsurfHooksJSON creates the Windsurf Cascade hooks.json content.
func GenerateWindsurfHooksJSON(handlerPath string) (string, error) {
	if err := validateHandlerPath(handlerPath); err != nil {
		return "", err
	}

	config := WindsurfHookConfig{
		Hooks: make(map[string][]WindsurfHookItem),
	}

	quotedPath := quotePathForShell(handlerPath)

	for _, hookType := range windsurfHookTypes {
		config.Hooks[hookType] = []WindsurfHookItem{{
			Command:    quotedPath + " hook --tool windsurf --event " + hookType,
			ShowOutput: false,
		}}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Windsurf hooks JSON: %w", err)
	}
	return string(data), nil
}

// geminiHookTypes contains all available hooks per https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/reference.md.
var geminiHookTypes = []string{
	"BeforeTool",
	"AfterTool",
	"BeforeAgent",
	"AfterAgent",
	"BeforeModel",
	"BeforeToolSelection",
	"AfterModel",
	"SessionStart",
	"SessionEnd",
	"Notification",
	"PreCompress",
}

// GenerateGeminiHooksJSON creates the Gemini CLI hooks configuration.
// Returns an error if the handler path contains unsafe characters.
func GenerateGeminiHooksJSON(handlerPath string) (map[string]any, error) {
	if err := validateHandlerPath(handlerPath); err != nil {
		return nil, err
	}

	cmd := handlerPath
	if runtime.GOOS == "windows" {
		cmd = handlerPath + ".exe"
	}

	quotedCmd := quotePathForShell(cmd)

	hooks := make(map[string]any)
	for _, hookType := range geminiHookTypes {
		hooks[hookType] = []map[string]any{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{
						"type":    "command",
						"command": quotedCmd + " hook --tool gemini --event " + hookType,
					},
				},
			},
		}
	}

	return hooks, nil
}
