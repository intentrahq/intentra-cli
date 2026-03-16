// Package hooks provides event normalization across AI coding tools.
// This file defines the normalizer interface. Event type constants are defined
// in pkg/models/event.go; aliases are provided here for package-level convenience.
package hooks

import "github.com/intentrahq/intentra-cli/pkg/models"

// NormalizedEventType is an alias for models.NormalizedEventType.
type NormalizedEventType = models.NormalizedEventType

const (
	EventSessionStart = models.EventSessionStart
	EventSessionEnd   = models.EventSessionEnd

	EventBeforePrompt  = models.EventBeforePrompt
	EventAfterResponse = models.EventAfterResponse
	EventAgentThought  = models.EventAgentThought

	EventBeforeTool = models.EventBeforeTool
	EventAfterTool  = models.EventAfterTool

	EventBeforeFileRead = models.EventBeforeFileRead
	EventAfterFileRead  = models.EventAfterFileRead
	EventBeforeFileEdit = models.EventBeforeFileEdit
	EventAfterFileEdit  = models.EventAfterFileEdit

	EventBeforeShell = models.EventBeforeShell
	EventAfterShell  = models.EventAfterShell

	EventBeforeMCP = models.EventBeforeMCP
	EventAfterMCP  = models.EventAfterMCP

	EventBeforeModel = models.EventBeforeModel
	EventAfterModel  = models.EventAfterModel

	EventToolSelection     = models.EventToolSelection
	EventPermissionRequest = models.EventPermissionRequest
	EventNotification      = models.EventNotification
	EventStop              = models.EventStop
	EventSubagentStart     = models.EventSubagentStart
	EventSubagentStop      = models.EventSubagentStop
	EventPreCompact        = models.EventPreCompact
	EventError             = models.EventError
	EventToolUseFailure    = models.EventToolUseFailure
	EventWorktreeSetup     = models.EventWorktreeSetup
	EventUnknown           = models.EventUnknown

	EventPostCompact            = models.EventPostCompact
	EventTeammateIdle           = models.EventTeammateIdle
	EventTaskCompleted          = models.EventTaskCompleted
	EventInstructionsLoaded     = models.EventInstructionsLoaded
	EventConfigChange           = models.EventConfigChange
	EventWorktreeCreate         = models.EventWorktreeCreate
	EventWorktreeRemove         = models.EventWorktreeRemove
	EventElicitation            = models.EventElicitation
	EventElicitationResult      = models.EventElicitationResult
	EventBeforeToolSelection    = models.EventBeforeToolSelection
	EventPreCompress            = models.EventPreCompress
	EventResponseWithTranscript = models.EventResponseWithTranscript
)

// Normalizer defines the interface for tool-specific event normalizers.
type Normalizer interface {
	// NormalizeEventType converts a tool-native event name (nativeType) to a
	// unified NormalizedEventType. Returns EventUnknown for unrecognized names.
	NormalizeEventType(nativeType string) NormalizedEventType
	// Tool returns the tool identifier string (e.g. "cursor", "claude").
	Tool() string
}

var normalizers = map[string]Normalizer{}

// RegisterNormalizer registers a normalizer for a specific tool.
func RegisterNormalizer(n Normalizer) {
	normalizers[n.Tool()] = n
}

// GetNormalizer returns the normalizer for the specified tool.
// Returns GenericNormalizer if no specific normalizer is registered.
func GetNormalizer(tool string) Normalizer {
	if n, ok := normalizers[tool]; ok {
		return n
	}
	return &GenericNormalizer{}
}

// tableNormalizer implements Normalizer using a simple mapping table.
// Tool-specific normalizers register themselves with this in init().
type tableNormalizer struct {
	tool    string
	mapping map[string]NormalizedEventType
}

// Tool returns the tool identifier.
func (n *tableNormalizer) Tool() string { return n.tool }

// NormalizeEventType converts a tool-native event name to a unified type.
func (n *tableNormalizer) NormalizeEventType(native string) NormalizedEventType {
	if normalized, ok := n.mapping[native]; ok {
		return normalized
	}
	return EventUnknown
}

// GenericNormalizer handles unknown tools by returning EventUnknown.
type GenericNormalizer struct{}

// Tool returns empty string for generic normalizer.
func (n *GenericNormalizer) Tool() string { return "" }

// NormalizeEventType returns EventUnknown for unrecognized events.
func (n *GenericNormalizer) NormalizeEventType(native string) NormalizedEventType {
	return EventUnknown
}

// toolMappings defines the event type mappings for all supported AI coding tools.
// Each tool maps its native event names to unified NormalizedEventType constants.
var toolMappings = map[string]map[string]NormalizedEventType{
	"cursor": {
		"sessionStart":         EventSessionStart,
		"sessionEnd":           EventSessionEnd,
		"beforeSubmitPrompt":   EventBeforePrompt,
		"afterAgentResponse":   EventAfterResponse,
		"afterAgentThought":    EventAgentThought,
		"beforeShellExecution": EventBeforeShell,
		"afterShellExecution":  EventAfterShell,
		"beforeMCPExecution":   EventBeforeMCP,
		"afterMCPExecution":    EventAfterMCP,
		"beforeTabFileRead":    EventBeforeFileRead,
		"beforeReadFile":       EventBeforeFileRead,
		"afterFileEdit":        EventAfterFileEdit,
		"afterTabFileEdit":     EventAfterFileEdit,
		"preToolUse":           EventBeforeTool,
		"postToolUse":          EventAfterTool,
		"postToolUseFailure":   EventToolUseFailure,
		"preCompact":           EventPreCompact,
		"subagentStart":        EventSubagentStart,
		"subagentStop":         EventSubagentStop,
		"stop":                 EventStop,
	},
	"claude": {
		"SessionStart":        EventSessionStart,
		"SessionEnd":          EventSessionEnd,
		"UserPromptSubmit":    EventBeforePrompt,
		"PreToolUse":          EventBeforeTool,
		"PostToolUse":         EventAfterTool,
		"PostToolUseFailure":  EventToolUseFailure,
		"PermissionRequest":   EventPermissionRequest,
		"Notification":        EventNotification,
		"Stop":                EventStop,
		"SubagentStart":       EventSubagentStart,
		"SubagentStop":        EventSubagentStop,
		"PreCompact":          EventPreCompact,
		"PostCompact":         EventPostCompact,
		"TeammateIdle":        EventTeammateIdle,
		"TaskCompleted":       EventTaskCompleted,
		"InstructionsLoaded":  EventInstructionsLoaded,
		"ConfigChange":        EventConfigChange,
		"WorktreeCreate":      EventWorktreeCreate,
		"WorktreeRemove":      EventWorktreeRemove,
		"Elicitation":         EventElicitation,
		"ElicitationResult":   EventElicitationResult,
	},
	"copilot": {
		"sessionStart":        EventSessionStart,
		"sessionEnd":          EventSessionEnd,
		"userPromptSubmitted": EventBeforePrompt,
		"preToolUse":          EventBeforeTool,
		"postToolUse":         EventAfterTool,
		"agentStop":           EventStop,
		"subagentStop":        EventSubagentStop,
		"errorOccurred":       EventError,
	},
	"windsurf": {
		"pre_user_prompt":                     EventBeforePrompt,
		"post_cascade_response":               EventAfterResponse,
		"post_cascade_response_with_transcript": EventResponseWithTranscript,
		"pre_read_code":                       EventBeforeFileRead,
		"post_read_code":                      EventAfterFileRead,
		"pre_write_code":                      EventBeforeFileEdit,
		"post_write_code":                     EventAfterFileEdit,
		"pre_run_command":                     EventBeforeShell,
		"post_run_command":                    EventAfterShell,
		"pre_mcp_tool_use":                    EventBeforeMCP,
		"post_mcp_tool_use":                   EventAfterMCP,
		"post_setup_worktree":                 EventWorktreeSetup,
	},
	"gemini": {
		"SessionStart":        EventSessionStart,
		"SessionEnd":          EventSessionEnd,
		"BeforeAgent":         EventBeforePrompt,
		"AfterAgent":          EventAfterResponse,
		"BeforeModel":         EventBeforeModel,
		"AfterModel":          EventAfterModel,
		"BeforeToolSelection": EventBeforeToolSelection,
		"BeforeTool":          EventBeforeTool,
		"AfterTool":           EventAfterTool,
		"PreCompress":         EventPreCompress,
		"Notification":        EventNotification,
	},
}

func init() {
	for tool, mapping := range toolMappings {
		RegisterNormalizer(&tableNormalizer{tool: tool, mapping: mapping})
	}
}

// IsStopEvent returns true if the event type marks the end of a scan.
// Each tool has exactly ONE designated terminal event to prevent duplicate scans.
//
// NOTE: Windsurf does not provide a dedicated "stop" hook. We use
// EventAfterResponse as the best available proxy, but this means scans may
// be incomplete if the session continues after the last observed response.
// Windsurf sessions that end without a final response will not generate a scan.
func IsStopEvent(eventType NormalizedEventType, tool string) bool {
	switch tool {
	case "windsurf":
		return eventType == EventAfterResponse
	case "copilot", "gemini":
		return eventType == EventSessionEnd
	default:
		return eventType == EventStop
	}
}

// IsSessionEndEvent returns true if this event carries session-end metadata
// that should be PATCHed onto the last scan (not trigger a new scan).
func IsSessionEndEvent(eventType NormalizedEventType, tool string) bool {
	if tool == "windsurf" || tool == "copilot" {
		return false
	}
	return eventType == EventSessionEnd
}

// IsLLMCallEvent delegates to models.IsLLMCallEvent.
func IsLLMCallEvent(eventType NormalizedEventType) bool {
	return models.IsLLMCallEvent(eventType)
}

// IsToolCallEvent delegates to models.IsToolCallEvent.
func IsToolCallEvent(eventType NormalizedEventType) bool {
	return models.IsToolCallEvent(eventType)
}
