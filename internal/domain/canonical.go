// Package domain defines canonical request/response types.
// OpenAI Chat Completions schema is used as the canonical internal format
// for all cross-provider translations.
package domain

import "encoding/json"

// ─── Request Types ──────────────────────────────────────────────────────────

// ChatCompletionRequest is the canonical internal representation of a chat
// completion request. It mirrors the OpenAI Chat Completions API schema.
type ChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	N                *int            `json:"n,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Stop             json.RawMessage `json:"stop,omitempty"` // string or []string
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       json.RawMessage `json:"tool_choice,omitempty"` // string or object
	User             string          `json:"user,omitempty"`

	// Internal routing metadata — never serialized to upstream providers.
	OriginalModel  string     `json:"-"`
	IdempotencyKey string     `json:"-"`
	SessionID      string     `json:"-"`
	UserID         string     `json:"-"`
	ClientType     ClientType `json:"-"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role       Role            `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // string or []ContentPart
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
}

// ContentPart represents a multi-part content block (text, image_url, etc.).
type ContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *ImageURL    `json:"image_url,omitempty"`
}

// ImageURL holds a URL and optional detail level for image content parts.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Tool represents a function/tool definition in OpenAI format.
type Tool struct {
	Type     string   `json:"type"` // always "function"
	Function Function `json:"function"`
}

// Function describes a callable function tool.
type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
}

// ToolCall represents a model-generated call to a tool/function.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name and JSON-encoded arguments for a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ─── Response Types ──────────────────────────────────────────────────────────

// ChatCompletionResponse is the canonical non-streaming response.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice represents one completion candidate.
type Choice struct {
	Index        int          `json:"index"`
	Message      ChatMessage  `json:"message"`
	FinishReason FinishReason `json:"finish_reason"`
}

// Usage reports token consumption for a request/response pair.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ─── Streaming Types ─────────────────────────────────────────────────────────

// StreamChunk is a single SSE data payload in a streaming completion.
type StreamChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// StreamChoice is a single delta within a streaming chunk.
type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        Delta        `json:"delta"`
	FinishReason FinishReason `json:"finish_reason"`
}

// Delta contains the incremental token content for a streaming choice.
type Delta struct {
	Role      Role       `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ─── Models Types ────────────────────────────────────────────────────────────

// Model represents an upstream model in OpenAI /v1/models format.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelList is the response body for GET /v1/models.
type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}
