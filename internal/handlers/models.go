package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
	"github.com/tnotstar/go-ya/internal/services"
)

// ModelsHandler handles GET /v1/models.
type ModelsHandler struct {
	provRepo *persistence.ProviderRepo
	keyStore *services.KeyStore
	registry *providers.Registry
	logger   *slog.Logger
}

// NewModelsHandler returns a ModelsHandler.
func NewModelsHandler(
	pr *persistence.ProviderRepo,
	ks *services.KeyStore,
	reg *providers.Registry,
	logger *slog.Logger,
) *ModelsHandler {
	return &ModelsHandler{provRepo: pr, keyStore: ks, registry: reg, logger: logger}
}

// Handle responds with an aggregated list of models from all enabled providers.
// A real API key is resolved per provider because remote providers (OpenAI,
// OpenRouter, ...) require auth for ListModels; providers without stored keys
// (e.g. local llama.cpp) are queried with an empty key instead of skipped.
func (h *ModelsHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	enabledProviders, err := h.provRepo.ListEnabled(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "models: list providers", slog.String("error", err.Error()))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list providers")
	}

	allModels := make([]domain.Model, 0)

	for _, p := range enabledProviders {
		apiKey, _, err := h.keyStore.GetKey(ctx, p.ID)
		if err != nil {
			if !errors.Is(err, domain.ErrKeyNotFound) {
				h.logger.WarnContext(ctx, "models: select key",
					slog.String("provider", p.Name),
					slog.String("error", err.Error()),
				)
				continue
			}
			// No keys stored: query unauthenticated (local providers).
			apiKey = ""
		}

		adapter, err := h.registry.Build(&p, apiKey)
		if err != nil {
			continue
		}

		models, err := h.listModelsForProvider(ctx, adapter, p.Name)
		if err != nil {
			h.logger.WarnContext(ctx, "models: provider error",
				slog.String("provider", p.Name),
				slog.String("error", err.Error()),
			)
			continue
		}
		allModels = append(allModels, models...)
	}

	return c.JSON(http.StatusOK, domain.ModelList{
		Object: "list",
		Data:   allModels,
	})
}

// listModelsForProvider calls ListModels with timeout handling.
func (h *ModelsHandler) listModelsForProvider(
	ctx context.Context,
	p providers.Provider,
	name string,
) ([]domain.Model, error) {
	models, err := p.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	// Tag models with the provider prefix so routing works transparently.
	for i := range models {
		models[i].OwnedBy = name
	}
	return models, nil
}
