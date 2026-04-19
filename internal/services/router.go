package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
)

// RoutingResult carries the resolved provider and metadata for a routed request.
type RoutingResult struct {
	Provider     providers.Provider
	ProviderName string
	Model        string // model name with prefix stripped (sent upstream)
}

// RouterService resolves the upstream provider for a given model name using
// prefix-based routing and selects an active API key via KeyStore.
//
// Prefix convention:
//   - "openai:gpt-4o"                          → provider openai, model gpt-4o
//   - "openrouter:anthropic/claude-3-5-sonnet" → provider openrouter, model anthropic/claude-3-5-sonnet
//   - "gemini:gemini-2.0-flash"                → provider gemini, model gemini-2.0-flash
//   - "abacus:some-model"                      → provider abacus, model some-model
//   - "local:llama3"                           → provider llamacpp, model llama3
//   - "gpt-4o" (no prefix)                    → uses default provider from config
type RouterService struct {
	providerRepo *persistence.ProviderRepo
	keyStore     *KeyStore
	registry     *providers.Registry
	defaultProvider string
	maxRetries   int
}

// NewRouterService returns a new RouterService.
func NewRouterService(
	pr *persistence.ProviderRepo,
	ks *KeyStore,
	reg *providers.Registry,
	defaultProvider string,
	maxRetries int,
) *RouterService {
	return &RouterService{
		providerRepo:    pr,
		keyStore:        ks,
		registry:        reg,
		defaultProvider: defaultProvider,
		maxRetries:      maxRetries,
	}
}

// Route resolves the provider and model for the given model string.
// It tries up to maxRetries keys on rate-limit/auth errors before giving up.
// Note: Route is lower-level than RouteWithFallback; prefer RouteWithFallback in handlers.
func (r *RouterService) Route(ctx context.Context, modelWithPrefix string) (*RoutingResult, error) {
	providerName, model := parseModelPrefix(modelWithPrefix, r.defaultProvider)

	dbProvider, err := r.providerRepo.GetByName(ctx, providerName)
	if err != nil {
		return nil, fmt.Errorf("router: %w: %s", domain.ErrProviderNotFound, providerName)
	}
	if !dbProvider.Enabled {
		return nil, fmt.Errorf("router: provider %q is disabled", providerName)
	}

	apiKey, _, err := r.keyStore.GetKey(ctx, dbProvider.ID)
	if err != nil {
		return nil, err
	}

	provider, err := r.registry.Build(dbProvider, apiKey)
	if err != nil {
		return nil, fmt.Errorf("router: build provider: %w", err)
	}

	return &RoutingResult{
		Provider:     provider,
		ProviderName: dbProvider.Name,
		Model:        model,
	}, nil
}

// RouteWithFallback attempts the primary provider and falls back on rate-limit/auth errors.
// The call function receives a context.Context and the resolved RoutingResult.
func (r *RouterService) RouteWithFallback(
	ctx context.Context,
	modelWithPrefix string,
	call func(ctx context.Context, res *RoutingResult) error,
) error {
	providerName, model := parseModelPrefix(modelWithPrefix, r.defaultProvider)

	dbProvider, err := r.providerRepo.GetByName(ctx, providerName)
	if err != nil {
		return fmt.Errorf("router: %w: %s", domain.ErrProviderNotFound, providerName)
	}

	keys, err := r.keyStore.keyRepo.ListEnabledByProvider(ctx, dbProvider.ID)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: provider %s", domain.ErrKeyNotFound, providerName)
	}

	retries := min(r.maxRetries, len(keys))
	for attempt := 0; attempt < retries; attempt++ {
		apiKey, selectedKey, err := r.keyStore.GetKey(ctx, dbProvider.ID)
		if err != nil {
			return err
		}

		prov, err := r.registry.Build(dbProvider, apiKey)
		if err != nil {
			return fmt.Errorf("router: build provider: %w", err)
		}

		res := &RoutingResult{
			Provider:     prov,
			ProviderName: dbProvider.Name,
			Model:        model,
		}

		callErr := call(ctx, res)
		if callErr == nil {
			return nil
		}

		// On rate-limit or invalid key, disable the key and retry with the next.
		if errors.Is(callErr, domain.ErrRateLimited) || errors.Is(callErr, domain.ErrInvalidKey) {
			_ = r.keyStore.DisableKey(ctx, selectedKey.ID)
			continue
		}
		return callErr
	}

	return fmt.Errorf("router: all %d retries exhausted for provider %s", retries, providerName)
}

// parseModelPrefix splits "prefix:model" into (providerName, model).
// Alias "local" maps to "llamacpp". If no prefix, defaultProvider is used.
func parseModelPrefix(modelWithPrefix, defaultProvider string) (providerName, model string) {
	idx := strings.Index(modelWithPrefix, ":")
	if idx < 0 {
		return defaultProvider, modelWithPrefix
	}
	prefix := modelWithPrefix[:idx]
	model = modelWithPrefix[idx+1:]

	// Normalize known prefix aliases.
	switch prefix {
	case "local":
		prefix = "llamacpp"
	}
	return prefix, model
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
