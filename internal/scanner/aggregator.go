// Package scanner provides event aggregation and scan management for Intentra.
// It handles grouping events into scans, calculating metrics like token usage
// and cost estimates, and persisting scan data locally.
package scanner

import (
	"sort"
	"strings"

	"github.com/intentrahq/intentra-cli/pkg/models"
)

// modelPricing contains pricing per token (in USD) for various models.
// Aligned with backend MODEL_PRICING in handlers/scans.py.
// Keys are prefixes that match model strings via strings.HasPrefix.
// Values represent cost per 1K tokens in USD.
var modelPricing = map[string]float64{
	"claude-opus-4.5":               0.011,  // $0.011 per 1K tokens
	"claude-sonnet-4.5":             0.0066, // $0.0066 per 1K tokens
	"claude-haiku-4.5":              0.0022, // $0.0022 per 1K tokens
	"claude-4.5-opus-high-thinking": 0.015,  // $0.015 per 1K tokens
	"claude-opus-4":                 0.033,  // $0.033 per 1K tokens
	"claude-sonnet-4":               0.0066, // $0.0066 per 1K tokens
	"claude-3-5-sonnet":             0.003,  // $0.003 per 1K tokens
	"claude-3-opus":                 0.015,  // $0.015 per 1K tokens
	"claude-3-haiku":                0.00025, // $0.00025 per 1K tokens
	"gemini-3-pro":                  0.005,   // $0.005 per 1K tokens
	"gemini-3-flash":                0.00125, // $0.00125 per 1K tokens
	"gemini-2.5-pro":                0.00388, // $0.00388 per 1K tokens
	"gemini-2.0-flash":              0.00019, // $0.00019 per 1K tokens
	"gemini-1.5-pro":                0.00125, // $0.00125 per 1K tokens
	"gemini-1.5-flash":              0.000075, // $0.000075 per 1K tokens
	"gpt-5.2-pro":                   0.0651,  // $0.0651 per 1K tokens
	"gpt-5.2":                       0.00543, // $0.00543 per 1K tokens
	"o3-pro":                        0.038,   // $0.038 per 1K tokens
	"o3":                            0.0038,  // $0.0038 per 1K tokens
	"o1-mini":                       0.003,   // $0.003 per 1K tokens
	"o1":                            0.0285,  // $0.0285 per 1K tokens
	"gpt-4o":                        0.005,   // $0.005 per 1K tokens
	"gpt-4":                         0.03,    // $0.03 per 1K tokens
	"gpt-3.5-turbo":                 0.0005,  // $0.0005 per 1K tokens
}

// toolPricingMultipliers applies tool-specific cost adjustments.
// Aligned with backend TOOL_PRICING_MULTIPLIERS.
var toolPricingMultipliers = map[string]float64{
	"cursor":   1.0,
	"windsurf": 1.2,
	"copilot":  1.0,
	"claude":   1.0,
	"gemini":   1.0,
}

// AggregateEvents groups events by conversation into scans.
func AggregateEvents(events []models.Event) []models.Scan {
	// Group by conversation ID
	byConversation := make(map[string][]models.Event)
	for _, e := range events {
		if e.ConversationID == "" {
			continue
		}
		byConversation[e.ConversationID] = append(byConversation[e.ConversationID], e)
	}

	var scans []models.Scan
	for convID, convEvents := range byConversation {
		// Sort by timestamp
		sort.Slice(convEvents, func(i, j int) bool {
			return convEvents[i].Timestamp.Before(convEvents[j].Timestamp)
		})

		scan := createScan(convID, convEvents)
		scans = append(scans, scan)
	}

	// Sort scans by start time
	sort.Slice(scans, func(i, j int) bool {
		return scans[i].StartTime.Before(scans[j].StartTime)
	})

	return scans
}

func createScan(conversationID string, events []models.Event) models.Scan {
	scan := models.Scan{
		ConversationID: conversationID,
		Status:         models.ScanStatusPending,
		Events:         events,
	}

	// Generate ID from conversation ID and start time
	if len(events) > 0 {
		scan.StartTime = events[0].Timestamp
		scan.EndTime = events[len(events)-1].Timestamp

		scan.ID = models.GenerateScanID(conversationID, scan.StartTime)
	}

	for _, e := range events {
		scan.InputTokens += e.InputTokens
		scan.OutputTokens += e.OutputTokens
		scan.ThinkingTokens += e.ThinkingTokens

		eventType := models.NormalizedEventType(e.NormalizedType)
		if models.IsLLMCallEvent(eventType) {
			scan.LLMCalls++
		}
		if models.IsToolCallEvent(eventType) {
			scan.ToolCalls++
		}
	}

	scan.TotalTokens = scan.InputTokens + scan.OutputTokens + scan.ThinkingTokens
	scan.EstimatedCost = EstimateCost(scan.TotalTokens, getModel(events), getTool(events))

	return scan
}

func getModel(events []models.Event) string {
	for _, e := range events {
		if e.Model != "" {
			return e.Model
		}
	}
	return "claude-sonnet-4.5"
}

func getTool(events []models.Event) string {
	for _, e := range events {
		if e.Tool != "" {
			return e.Tool
		}
	}
	return "cursor"
}

// sortedModelPrefixes contains model pricing keys sorted by length descending,
// ensuring the longest (most specific) prefix always matches first.
var sortedModelPrefixes []string

func init() {
	sortedModelPrefixes = make([]string, 0, len(modelPricing))
	for prefix := range modelPricing {
		sortedModelPrefixes = append(sortedModelPrefixes, prefix)
	}
	sort.Slice(sortedModelPrefixes, func(i, j int) bool {
		return len(sortedModelPrefixes[i]) > len(sortedModelPrefixes[j])
	})
}

// EstimateCost calculates the estimated cost for a given number of tokens and model.
// Falls back to a default price of $0.005/1K tokens if the model is not recognized.
// Applies tool-specific pricing multipliers when tool is provided.
func EstimateCost(tokens int, model string, tool ...string) float64 {
	var basePrice float64
	matched := false
	for _, prefix := range sortedModelPrefixes {
		if strings.HasPrefix(model, prefix) {
			basePrice = modelPricing[prefix]
			matched = true
			break
		}
	}
	if !matched {
		basePrice = 0.005
	}
	multiplier := 1.0
	if len(tool) > 0 {
		if m, ok := toolPricingMultipliers[tool[0]]; ok {
			multiplier = m
		}
	}
	return float64(tokens) / 1000.0 * basePrice * multiplier
}


// AggregateFilesModified builds per-file edit statistics from a slice of events.
func AggregateFilesModified(events []models.Event) []map[string]any {
	type fileStats struct {
		linesAdded   int
		linesRemoved int
		editCount    int
		isNew        bool
		seenBefore   bool
	}

	stats := make(map[string]*fileStats)

	for _, ev := range events {
		if ev.FilePath == "" {
			continue
		}

		path := ev.FilePath
		s, exists := stats[path]
		if !exists {
			s = &fileStats{}
			stats[path] = s
		}

		switch models.NormalizedEventType(ev.NormalizedType) {
		case models.EventBeforeFileEdit:
			s.seenBefore = true
		case models.EventAfterFileEdit:
			s.editCount++
			s.linesAdded += ev.OutputTokens / 15
			if !s.seenBefore {
				s.isNew = true
			}
		}
	}

	if len(stats) == 0 {
		return nil
	}

	var result []map[string]any
	for path, s := range stats {
		if s.editCount == 0 {
			continue
		}
		entry := map[string]any{
			"file_path":     models.SanitizePath(path),
			"is_new_file":   s.isNew,
			"lines_added":   s.linesAdded,
			"lines_removed": s.linesRemoved,
			"edit_count":    s.editCount,
		}
		result = append(result, entry)
	}

	return result
}

