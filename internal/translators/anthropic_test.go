package translators_test

import (
	"encoding/json"
	"testing"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/translators"
)

func TestAnthropicToCanonical_BasicMessage(t *testing.T) {
	req := &translators.AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []translators.AnthropicMessage{
			{Role: "user", Content: translators.AnthropicContent(mustJSON(t, "Hello, Claude!"))},
		},
	}

	got, err := translators.AnthropicToCanonical(req)
	if err != nil {
		t.Fatalf("AnthropicToCanonical() error = %v", err)
	}

	if got.Model != req.Model {
		t.Errorf("model: got %q, want %q", got.Model, req.Model)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages: got %d, want 1", len(got.Messages))
	}
	if got.Messages[0].Role != domain.RoleUser {
		t.Errorf("role: got %q, want %q", got.Messages[0].Role, domain.RoleUser)
	}
	if *got.MaxTokens != 1024 {
		t.Errorf("max_tokens: got %d, want 1024", *got.MaxTokens)
	}
}

func TestAnthropicToCanonical_SystemMessage(t *testing.T) {
	req := &translators.AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 512,
		System:    "You are a helpful assistant.",
		Messages: []translators.AnthropicMessage{
			{Role: "user", Content: translators.AnthropicContent(mustJSON(t, "Hi"))},
		},
	}

	got, err := translators.AnthropicToCanonical(req)
	if err != nil {
		t.Fatalf("AnthropicToCanonical() error = %v", err)
	}

	// System message should be prepended.
	if len(got.Messages) != 2 {
		t.Fatalf("messages: got %d, want 2 (system + user)", len(got.Messages))
	}
	if got.Messages[0].Role != domain.RoleSystem {
		t.Errorf("first message role: got %q, want system", got.Messages[0].Role)
	}
}

func TestAnthropicToCanonical_ToolConversion(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	req := &translators.AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 200,
		Messages: []translators.AnthropicMessage{
			{Role: "user", Content: translators.AnthropicContent(mustJSON(t, "Search something"))},
		},
		Tools: []translators.AnthropicTool{
			{Name: "search", Description: "Web search", InputSchema: schema},
		},
	}

	got, err := translators.AnthropicToCanonical(req)
	if err != nil {
		t.Fatalf("AnthropicToCanonical() error = %v", err)
	}

	if len(got.Tools) != 1 {
		t.Fatalf("tools: got %d, want 1", len(got.Tools))
	}
	if got.Tools[0].Function.Name != "search" {
		t.Errorf("tool name: got %q, want search", got.Tools[0].Function.Name)
	}
	if got.Tools[0].Type != "function" {
		t.Errorf("tool type: got %q, want function", got.Tools[0].Type)
	}
}

func TestCanonicalToAnthropic_BasicResponse(t *testing.T) {
	content, _ := json.Marshal("Hello from assistant")
	resp := &domain.ChatCompletionResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4o",
		Choices: []domain.Choice{
			{
				Index:        0,
				FinishReason: domain.FinishReasonStop,
				Message: domain.ChatMessage{
					Role:    domain.RoleAssistant,
					Content: content,
				},
			},
		},
		Usage: &domain.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	got := translators.CanonicalToAnthropic(resp)

	if got.Type != "message" {
		t.Errorf("type: got %q, want message", got.Type)
	}
	if got.Role != "assistant" {
		t.Errorf("role: got %q, want assistant", got.Role)
	}
	if got.StopReason != "end_turn" {
		t.Errorf("stop_reason: got %q, want end_turn", got.StopReason)
	}
	if got.Usage.InputTokens != 10 {
		t.Errorf("input_tokens: got %d, want 10", got.Usage.InputTokens)
	}
}

func TestCanonicalToAnthropic_FinishReasonMapping(t *testing.T) {
	cases := []struct {
		inReason  domain.FinishReason
		wantReason string
	}{
		{domain.FinishReasonStop, "end_turn"},
		{domain.FinishReasonLength, "max_tokens"},
		{domain.FinishReasonToolCalls, "tool_use"},
	}

	for _, tc := range cases {
		content, _ := json.Marshal("")
		resp := &domain.ChatCompletionResponse{
			Choices: []domain.Choice{
				{FinishReason: tc.inReason, Message: domain.ChatMessage{Content: content}},
			},
		}
		got := translators.CanonicalToAnthropic(resp)
		if got.StopReason != tc.wantReason {
			t.Errorf("finish_reason %q: got stop_reason %q, want %q",
				tc.inReason, got.StopReason, tc.wantReason)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
