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
	return &OpenAIProvider{
		openAILikeProvider: newOpenAILikeProvider(
			openAIBaseURL,
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
	return p.openAILikeProvider.ChatCompletion(ctx, req)
}

// ChatCompletionStream delegates to the embedded adapter.
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req *domain.ChatCompletionRequest) (<-chan StreamEvent, error) {
	return p.openAILikeProvider.ChatCompletionStream(ctx, req)
}
