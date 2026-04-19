package providers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/domain"
)

// Registry creates Provider instances from domain.Provider records.
// It is the factory layer between the DB model and the live adapter.
type Registry struct {
	cfg    *config.Config
	client *http.Client
}

// NewRegistry returns a Registry using the given config and shared HTTP client.
func NewRegistry(cfg *config.Config) *Registry {
	return &Registry{
		cfg: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Build constructs a Provider adapter for the given domain.Provider record
// using the supplied plaintext API key.
func (r *Registry) Build(p *domain.Provider, apiKey string) (Provider, error) {
	switch p.Type {
	case domain.ProviderTypeOpenAI:
		return NewOpenAI(apiKey, r.client), nil

	case domain.ProviderTypeOpenRouter:
		return NewOpenRouter(
			apiKey,
			r.cfg.OpenRouterReferer,
			r.cfg.OpenRouterTitle,
			r.cfg.ModelCacheTTL,
			r.client,
		), nil

	case domain.ProviderTypeGemini:
		return NewGemini(apiKey, r.client), nil

	case domain.ProviderTypeAbacus:
		return NewAbacus(
			p.BaseURL,
			apiKey,
			p.AuthHeaderName,
			"", // use default format "%s"
			r.client,
		), nil

	case domain.ProviderTypeLlamaCPP:
		return NewLlamaCPP(p.BaseURL, apiKey), nil

	case domain.ProviderTypeMock:
		return NewMock(), nil

	default:
		return nil, fmt.Errorf("registry: unknown provider type %q", p.Type)
	}
}
