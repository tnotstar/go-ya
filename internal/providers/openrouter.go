package providers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/tnotstar/go-ya/internal/domain"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterProvider adapts the OpenRouter API (OpenAI-compatible with attribution headers).
// It caches the model list for a configurable TTL to avoid hammering the /models endpoint.
type OpenRouterProvider struct {
	*openAILikeProvider

	cacheMu      sync.Mutex
	cachedModels []domain.Model
	cacheExpiry  time.Time
	cacheTTL     time.Duration
}

// NewOpenRouter returns an OpenRouter adapter.
// referer and title are used for attribution headers as per OpenRouter docs.
// cacheTTL controls how long the model list is cached (0 = use 5m default).
func NewOpenRouter(apiKey, referer, title string, cacheTTL time.Duration, client *http.Client) *OpenRouterProvider {
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}
	extra := map[string]string{
		"HTTP-Referer": referer,
		"X-Title":      title,
	}
	return &OpenRouterProvider{
		openAILikeProvider: newOpenAILikeProvider(
			openRouterBaseURL,
			"Authorization", "Bearer %s",
			extra,
			client,
			domain.ProviderTypeOpenRouter,
			apiKey,
		),
		cacheTTL: cacheTTL,
	}
}

// ListModels returns available models, using a TTL cache to reduce API calls.
func (p *OpenRouterProvider) ListModels(ctx context.Context) ([]domain.Model, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	if time.Now().Before(p.cacheExpiry) && len(p.cachedModels) > 0 {
		return p.cachedModels, nil
	}

	models, err := p.openAILikeProvider.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	p.cachedModels = models
	p.cacheExpiry = time.Now().Add(p.cacheTTL)
	return models, nil
}

// ChatCompletion passes through to the embedded adapter.
func (p *OpenRouterProvider) ChatCompletion(ctx context.Context, req *domain.ChatCompletionRequest) (*domain.ChatCompletionResponse, error) {
	return p.openAILikeProvider.ChatCompletion(ctx, req)
}

// ChatCompletionStream passes through to the embedded adapter.
func (p *OpenRouterProvider) ChatCompletionStream(ctx context.Context, req *domain.ChatCompletionRequest) (<-chan StreamEvent, error) {
	return p.openAILikeProvider.ChatCompletionStream(ctx, req)
}
