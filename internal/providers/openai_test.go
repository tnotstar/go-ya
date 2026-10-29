package providers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/providers"
)

// stubOpenAI returns an httptest server that answers any chat completion with
// a minimal valid response and records the last request body received.
func stubOpenAI(t *testing.T) (*httptest.Server, func() map[string]any) {
	t.Helper()
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &lastBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-x","object":"chat.completion","created":1,` +
			`"model":"stub","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() map[string]any { return lastBody }
}

func toolRequest() *domain.ChatCompletionRequest {
	return &domain.ChatCompletionRequest{
		Model: "stub-model",
		Messages: []domain.ChatMessage{
			{Role: domain.RoleUser, Content: json.RawMessage(`"di ok"`)},
		},
		Tools: []domain.Tool{{
			Type: "function",
			Function: domain.Function{
				Name:        "read_file",
				Description: "Lee un archivo",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		}},
	}
}

// The gpt-6 family rejects function tools on chat/completions unless
// reasoning_effort is "none"; the adapter must force it.
func TestOpenAIChatCompletionForcesReasoningEffortNoneWithTools(t *testing.T) {
	srv, lastBody := stubOpenAI(t)
	p := providers.NewOpenAIWithBaseURL(srv.URL, "test-key", srv.Client())

	req := toolRequest()
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if got := lastBody()["reasoning_effort"]; got != "none" {
		t.Errorf("reasoning_effort = %v, want %q", got, "none")
	}
}

func TestOpenAIChatCompletionStreamForcesReasoningEffortNoneWithTools(t *testing.T) {
	got := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		got <- body
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	ch, err := providers.NewOpenAIWithBaseURL(srv.URL, "test-key", srv.Client()).
		ChatCompletionStream(context.Background(), toolRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	for range ch {
	}

	body := <-got
	if v := body["reasoning_effort"]; v != "none" {
		t.Errorf("reasoning_effort = %v, want %q", v, "none")
	}
}

func TestOpenAIChatCompletionOmitsReasoningEffortWithoutTools(t *testing.T) {
	srv, lastBody := stubOpenAI(t)
	p := providers.NewOpenAIWithBaseURL(srv.URL, "test-key", srv.Client())

	req := toolRequest()
	req.Tools = nil
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if _, present := lastBody()["reasoning_effort"]; present {
		t.Errorf("reasoning_effort should be omitted without tools, got %v", lastBody()["reasoning_effort"])
	}
}

// The registry must honor base_url stored in the DB for openai providers.
func TestRegistryBuildOpenAIUsesProviderBaseURL(t *testing.T) {
	srv, lastBody := stubOpenAI(t)
	reg := providers.NewRegistry(&config.Config{})

	prov, err := reg.Build(&domain.Provider{
		Name:    "openai",
		Type:    domain.ProviderTypeOpenAI,
		BaseURL: srv.URL,
	}, "test-key")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := toolRequest()
	if _, err := prov.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if lastBody()["model"] != "stub-model" {
		t.Fatalf("request did not reach the stub server: %v", lastBody())
	}
	if got := lastBody()["reasoning_effort"]; got != "none" {
		t.Errorf("reasoning_effort = %v, want %q", got, "none")
	}
}
