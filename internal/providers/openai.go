// Package providers — OpenAI adapter.
package providers

import (
	"context"
	"net/http"

	"github.com/tnotstar/go-ya/internal/domain"
)

const openAIBaseURL = "https://api.openai.com/v1"

// OpenAIProvider adapts the OpenAI Chat Completions API.
// The canonical internal format IS the OpenAI format, so no translation is needed.
type OpenAIProvider struct {
	*openAILikeProvider
}

// NewOpenAI returns a new OpenAI adapter using the supplied API key.
func NewOpenAI(apiKey string, client *http.Client) *OpenAIProvider {
	return NewOpenAIWithBaseURL(openAIBaseURL, apiKey, client)
}

// NewOpenAIWithBaseURL returns an OpenAI adapter for a custom endpoint.
// An empty baseURL falls back to the public API.
func NewOpenAIWithBaseURL(baseURL, apiKey string, client *http.Client) *OpenAIProvider {
	if baseURL == "" {
		baseURL = openAIBaseURL
	}
	return &OpenAIProvider{
		openAILikeProvider: newOpenAILikeProvider(
			baseURL,
			"Authorization", "Bearer %s",
			nil,
			client,
			domain.ProviderTypeOpenAI,
			apiKey,
		),
	}
}

// ListModels delegates to the embedded OpenAI-like adapter.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]domain.Model, error) {
	return p.openAILikeProvider.ListModels(ctx)
}

// ChatCompletion delegates to the embedded adapter (no translation required).
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *domain.ChatCompletionRequest) (*domain.ChatCompletionResponse, error) {
	constrainToolsReasoning(req)
	return p.openAILikeProvider.ChatCompletion(ctx, req)
}

// ChatCompletionStream delegates to the embedded adapter.
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req *domain.ChatCompletionRequest) (<-chan StreamEvent, error) {
	constrainToolsReasoning(req)
	return p.openAILikeProvider.ChatCompletionStream(ctx, req)
}

// constrainToolsReasoning works around an upstream restriction of the gpt-6
// family: function tools on /v1/chat/completions are rejected with HTTP 400
// ("Function tools with reasoning_effort are not supported ... or set
// reasoning_effort to 'none'") unless the effort is explicitly "none". The
// model reasons by default even when the field is omitted, so it must be forced.
func constrainToolsReasoning(req *domain.ChatCompletionRequest) {
	if len(req.Tools) > 0 {
		req.ReasoningEffort = "none"
	}
}
