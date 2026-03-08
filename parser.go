package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Top-level event from stream-json output
type StreamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	UUID    string          `json:"uuid,omitempty"`

	// For system init
	CWD       string   `json:"cwd,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	Model     string   `json:"model,omitempty"`
	Tools     []string `json:"tools,omitempty"`

	// For assistant/user messages
	Message *Message `json:"message,omitempty"`

	// For tool_result on user events
	ToolUseResult *ToolUseResult `json:"tool_use_result,omitempty"`

	// For rate_limit_event
	RateLimitInfo *RateLimitInfo `json:"rate_limit_info,omitempty"`

	// For result
	IsError      bool    `json:"is_error,omitempty"`
	DurationMS   int     `json:"duration_ms,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	Result       string  `json:"result,omitempty"`
	StopReason   string  `json:"stop_reason,omitempty"`
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
	Usage        *Usage  `json:"usage,omitempty"`
}

type Message struct {
	ID      string         `json:"id,omitempty"`
	Role    string         `json:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
	Usage   *Usage         `json:"usage,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking string `json:"thinking,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result (in user messages)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type ToolUseResult struct {
	Stdout      string `json:"stdout,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
	Interrupted bool   `json:"interrupted,omitempty"`
	IsImage     bool   `json:"isImage,omitempty"`
}

type RateLimitInfo struct {
	Status        string `json:"status,omitempty"`
	RateLimitType string `json:"rateLimitType,omitempty"`
}

type Usage struct {
	InputTokens              int     `json:"input_tokens,omitempty"`
	OutputTokens             int     `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens,omitempty"`
}

// ParseEvent parses a single line of stream-json output.
func ParseEvent(line []byte) (*StreamEvent, error) {
	var ev StreamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// UIEvent is what we send to the browser via SSE.
type UIEvent struct {
	Type string `json:"type"` // "init", "turn_start", "text", "thinking", "tool_use", "tool_result", "rate_limit", "result", "error", "status"

	// Common
	SessionID string `json:"session_id,omitempty"`

	// For text/thinking
	Content string `json:"content,omitempty"`

	// For tool_use
	ToolName  string `json:"tool_name,omitempty"`
	ToolID    string `json:"tool_id,omitempty"`
	ToolInput string `json:"tool_input,omitempty"` // summarized

	// For tool_result
	ParentToolID string `json:"parent_tool_id,omitempty"`
	IsError      bool   `json:"is_error,omitempty"`

	// For result
	DurationMS   int     `json:"duration_ms,omitempty"`
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	StopReason   string  `json:"stop_reason,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`

	// For init
	Model string `json:"model,omitempty"`
	CWD   string `json:"cwd,omitempty"`

	// For status
	Running bool `json:"running,omitempty"`
}

// TransformEvent converts a raw StreamEvent into UIEvents for the browser.
func TransformEvent(ev *StreamEvent) []UIEvent {
	switch ev.Type {
	case "system":
		return []UIEvent{{
			Type:      "init",
			SessionID: ev.SessionID,
			Model:     ev.Model,
			CWD:       ev.CWD,
		}}

	case "assistant":
		if ev.Message == nil {
			return nil
		}
		var events []UIEvent
		for _, block := range ev.Message.Content {
			switch block.Type {
			case "text":
				events = append(events, UIEvent{Type: "text", Content: block.Text})
			case "thinking":
				events = append(events, UIEvent{Type: "thinking", Content: block.Thinking})
			case "tool_use":
				events = append(events, UIEvent{
					Type:      "tool_use",
					ToolName:  block.Name,
					ToolID:    block.ID,
					ToolInput: summarizeToolInput(block.Name, block.Input),
				})
			}
		}
		return events

	case "user":
		if ev.Message == nil {
			return nil
		}
		var events []UIEvent
		for _, block := range ev.Message.Content {
			if block.Type == "tool_result" {
				content := block.Content
				if ev.ToolUseResult != nil {
					if ev.ToolUseResult.Stdout != "" {
						content = ev.ToolUseResult.Stdout
					}
					if ev.ToolUseResult.Stderr != "" {
						if content != "" {
							content += "\n"
						}
						content += ev.ToolUseResult.Stderr
					}
				}
				events = append(events, UIEvent{
					Type:         "tool_result",
					ParentToolID: block.ToolUseID,
					Content:      content,
					IsError:      block.IsError,
				})
			}
		}
		return events

	case "rate_limit_event":
		info := ""
		if ev.RateLimitInfo != nil {
			info = fmt.Sprintf("%s (%s)", ev.RateLimitInfo.Status, ev.RateLimitInfo.RateLimitType)
		}
		return []UIEvent{{Type: "rate_limit", Content: info}}

	case "result":
		ui := UIEvent{
			Type:         "result",
			Content:      ev.Result,
			DurationMS:   ev.DurationMS,
			TotalCostUSD: ev.TotalCostUSD,
			NumTurns:     ev.NumTurns,
			StopReason:   ev.StopReason,
			IsError:      ev.IsError,
		}
		if ev.Usage != nil {
			ui.InputTokens = ev.Usage.InputTokens
			ui.OutputTokens = ev.Usage.OutputTokens
		}
		return []UIEvent{ui}
	}
	return nil
}

// summarizeToolInput extracts key info from tool input for display.
func summarizeToolInput(toolName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return string(raw)
	}

	switch toolName {
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	case "Read":
		if p, ok := m["file_path"].(string); ok {
			return p
		}
	case "Write":
		if p, ok := m["file_path"].(string); ok {
			return p
		}
	case "Edit":
		if p, ok := m["file_path"].(string); ok {
			return p
		}
	case "Glob":
		if p, ok := m["pattern"].(string); ok {
			return p
		}
	case "Grep":
		if p, ok := m["pattern"].(string); ok {
			return p
		}
	}

	// Fallback: show first string value
	for _, v := range m {
		if s, ok := v.(string); ok {
			if len(s) > 120 {
				return s[:120] + "..."
			}
			return s
		}
	}
	return truncate(string(raw), 120)
}

// BuildContextEntry creates a lean text entry for the context file.
func BuildContextEntry(ev *StreamEvent) string {
	switch ev.Type {
	case "assistant":
		if ev.Message == nil {
			return ""
		}
		var parts []string
		for _, block := range ev.Message.Content {
			switch block.Type {
			case "text":
				if t := strings.TrimSpace(block.Text); t != "" {
					parts = append(parts, t)
				}
			case "tool_use":
				parts = append(parts, fmt.Sprintf("Tool(%s): %s", block.Name, summarizeToolInput(block.Name, block.Input)))
			}
			// Skip thinking blocks from context
		}
		if len(parts) > 0 {
			return "Assistant: " + strings.Join(parts, "\n") + "\n\n"
		}

	case "user":
		if ev.Message == nil || ev.ToolUseResult == nil {
			return ""
		}
		result := ev.ToolUseResult.Stdout
		if result == "" {
			result = ev.ToolUseResult.Stderr
		}
		lines := strings.Split(result, "\n")
		if len(lines) > 3 {
			lines = append(lines[:3], "...")
		}
		return "Result: " + strings.Join(lines, "\n") + "\n\n"
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
