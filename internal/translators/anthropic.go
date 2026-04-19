// Package translators implements protocol translation between downstream client
// formats and the canonical internal OpenAI schema.
package translators

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tnotstar/go-ya/internal/domain"
)

// ─── Anthropic Request Types ─────────────────────────────────────────────────

// AnthropicRequest is the request body for POST /v1/messages (Anthropic API).
type AnthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream,omitempty"`
	// Optional parameters.
	Temperature *float64            `json:"temperature,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	Stop        []string            `json:"stop_sequences,omitempty"`
	Tools       []AnthropicTool     `json:"tools,omitempty"`
	ToolChoice  *AnthropicToolChoice `json:"tool_choice,omitempty"`
	Metadata    *AnthropicMeta      `json:"metadata,omitempty"`
}

// AnthropicMessage is a message in the Anthropic Messages API format.
type AnthropicMessage struct {
	Role    string              `json:"role"` // "user" or "assistant"
	Content AnthropicContent    `json:"content"` // string or []AnthropicBlock
}

// AnthropicContent is either a plain string or an array of content blocks.
type AnthropicContent json.RawMessage

// AnthropicBlock represents a single content block within a message.
type AnthropicBlock struct {
	Type      string          `json:"type"` // "text", "tool_use", "tool_result", "image"
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`   // for tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
}

// AnthropicTool defines a tool in Anthropic format.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// AnthropicToolChoice specifies tool selection behavior.
type AnthropicToolChoice struct {
	Type string `json:"type"` // "auto", "any", "tool"
	Name string `json:"name,omitempty"`
}

// AnthropicMeta carries request metadata.
type AnthropicMeta struct {
	UserID string `json:"user_id,omitempty"`
}

// ─── Anthropic Response Types ─────────────────────────────────────────────────

// AnthropicResponse is the non-streaming Anthropic Messages API response.
type AnthropicResponse struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"` // "message"
	Role         string           `json:"role"` // "assistant"
	Content      []AnthropicBlock `json:"content"`
	Model        string           `json:"model"`
	StopReason   string           `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Usage        AnthropicUsage   `json:"usage"`
}

// AnthropicUsage carries token counts in Anthropic format.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ─── Anthropic Stream Event Types ────────────────────────────────────────────

// AnthropicStreamEvent is a single SSE event in Anthropic streaming format.
type AnthropicStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta *AnthropicDelta `json:"delta,omitempty"`
	// Used in message_start.
	Message *AnthropicResponse `json:"message,omitempty"`
	// Used in content_block_start.
	ContentBlock *AnthropicBlock `json:"content_block,omitempty"`
	// Used in message_delta for usage.
	Usage *AnthropicUsage `json:"usage,omitempty"`
}

// AnthropicDelta carries an incremental content update.
type AnthropicDelta struct {
	Type        string `json:"type"` // "text_delta", "input_json_delta"
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// ─── ToCanonical: Anthropic → OpenAI ─────────────────────────────────────────

// AnthropicToCanonical converts an Anthropic Messages request to canonical format.
func AnthropicToCanonical(req *AnthropicRequest) (*domain.ChatCompletionRequest, error) {
	canonical := &domain.ChatCompletionRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   &req.MaxTokens,
	}
	if req.MaxTokens == 0 {
		defaultMax := 1024
		canonical.MaxTokens = &defaultMax
	}

	// Translate stop_sequences → stop (JSON array).
	if len(req.Stop) > 0 {
		b, _ := json.Marshal(req.Stop)
		canonical.Stop = b
	}

	// Prepend system message if present.
	var messages []domain.ChatMessage
	if req.System != "" {
		sysContent, _ := json.Marshal(req.System)
		messages = append(messages, domain.ChatMessage{
			Role:    domain.RoleSystem,
			Content: sysContent,
		})
	}

	// Convert Anthropic messages to canonical.
	for _, m := range req.Messages {
		msg, err := anthropicMsgToCanonical(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	canonical.Messages = messages

	// Convert tools.
	if len(req.Tools) > 0 {
		tools := make([]domain.Tool, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, domain.Tool{
				Type: "function",
				Function: domain.Function{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
		canonical.Tools = tools
	}

	// Convert tool_choice.
	if tc := req.ToolChoice; tc != nil {
		var oaiChoice any
		switch tc.Type {
		case "auto":
			oaiChoice = "auto"
		case "any":
			oaiChoice = "required"
		case "tool":
			oaiChoice = map[string]any{
				"type": "function",
				"function": map[string]string{"name": tc.Name},
			}
		}
		if oaiChoice != nil {
			b, _ := json.Marshal(oaiChoice)
			canonical.ToolChoice = b
		}
	}

	// Extract user_id from metadata.
	if req.Metadata != nil {
		canonical.UserID = req.Metadata.UserID
	}

	return canonical, nil
}

// anthropicMsgToCanonical converts a single Anthropic message to canonical.
func anthropicMsgToCanonical(m AnthropicMessage) (domain.ChatMessage, error) {
	msg := domain.ChatMessage{}
	switch m.Role {
	case "user":
		msg.Role = domain.RoleUser
	case "assistant":
		msg.Role = domain.RoleAssistant
	default:
		return msg, fmt.Errorf("anthropic: unknown role %q", m.Role)
	}

	raw := json.RawMessage(m.Content)

	// Try plain string first.
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		content, _ := json.Marshal(text)
		msg.Content = content
		return msg, nil
	}

	// Otherwise, parse content blocks.
	var blocks []AnthropicBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return msg, fmt.Errorf("anthropic: parse content: %w", err)
	}

	var textParts []string
	var toolCalls []domain.ToolCall

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			toolCalls = append(toolCalls, domain.ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: domain.FunctionCall{
					Name:      b.Name,
					Arguments: string(args),
				},
			})
		case "tool_result":
			// Tool results map to a "tool" role message with tool_call_id.
			// Handled by the caller inserting separate messages.
			// Here, embed as text.
			var resultText string
			if err := json.Unmarshal(b.Content, &resultText); err == nil {
				textParts = append(textParts, resultText)
			}
			msg.ToolCallID = b.ToolUseID
		}
	}

	if len(textParts) > 0 {
		content, _ := json.Marshal(strings.Join(textParts, ""))
		msg.Content = content
	}
	msg.ToolCalls = toolCalls

	return msg, nil
}

// ─── FromCanonical: OpenAI → Anthropic ───────────────────────────────────────

// CanonicalToAnthropic converts a canonical ChatCompletionResponse to Anthropic format.
func CanonicalToAnthropic(resp *domain.ChatCompletionResponse) *AnthropicResponse {
	ar := &AnthropicResponse{
		ID:    "msg_" + uuid.NewString(),
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		ar.StopReason = mapFinishReasonToAnthropic(choice.FinishReason)

		// Text content block.
		var text string
		_ = json.Unmarshal(choice.Message.Content, &text)
		if text != "" {
			ar.Content = append(ar.Content, AnthropicBlock{Type: "text", Text: text})
		}

		// Tool use blocks.
		for _, tc := range choice.Message.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			ar.Content = append(ar.Content, AnthropicBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: args,
			})
		}
	}

	if resp.Usage != nil {
		ar.Usage = AnthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	return ar
}

// ─── Streaming Translation: SSE OpenAI chunks → Anthropic events ─────────────

// AnthropicStreamState tracks mutable state during an Anthropic stream translation.
type AnthropicStreamState struct {
	opened      bool
	blockOpened bool
	inputTokens int
}

// CanonicalChunkToAnthropicEvents converts an OpenAI StreamChunk into
// one or more Anthropic SSE events. Returns the events in emission order.
// The state parameter is updated in-place across calls.
func CanonicalChunkToAnthropicEvents(
	chunk *domain.StreamChunk,
	state *AnthropicStreamState,
) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent

	if !state.opened {
		// Emit message_start with skeleton message.
		events = append(events, AnthropicStreamEvent{
			Type: "message_start",
			Message: &AnthropicResponse{
				ID:   "msg_" + uuid.NewString(),
				Type: "message",
				Role: "assistant",
			},
		})
		// Emit first content_block_start.
		events = append(events, AnthropicStreamEvent{
			Type:         "content_block_start",
			Index:        0,
			ContentBlock: &AnthropicBlock{Type: "text", Text: ""},
		})
		state.opened = true
		state.blockOpened = true
	}

	for _, c := range chunk.Choices {
		if c.Delta.Content != "" {
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: c.Index,
				Delta: &AnthropicDelta{
					Type: "text_delta",
					Text: c.Delta.Content,
				},
			})
		}

		if c.FinishReason != domain.FinishReasonNull {
			// Close the content block.
			events = append(events, AnthropicStreamEvent{Type: "content_block_stop", Index: 0})
			// Emit message_delta with stop_reason.
			events = append(events, AnthropicStreamEvent{
				Type: "message_delta",
				Delta: &AnthropicDelta{
					Type:       "message_delta",
					StopReason: mapFinishReasonToAnthropic(c.FinishReason),
				},
				Usage: &AnthropicUsage{
					OutputTokens: 0, // populated from usage if available
				},
			})
			// Emit message_stop.
			events = append(events, AnthropicStreamEvent{Type: "message_stop"})
		}
	}

	return events
}

// mapFinishReasonToAnthropic converts canonical FinishReason to Anthropic stop_reason.
func mapFinishReasonToAnthropic(r domain.FinishReason) string {
	switch r {
	case domain.FinishReasonStop:
		return "end_turn"
	case domain.FinishReasonLength:
		return "max_tokens"
	case domain.FinishReasonToolCalls:
		return "tool_use"
	default:
		return ""
	}
}

// nowUnix returns the current Unix timestamp. Extracted for test substitution.
func nowUnix() int64 { return time.Now().Unix() }
