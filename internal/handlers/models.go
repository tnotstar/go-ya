package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
)

// ModelsHandler handles GET /v1/models.
type ModelsHandler struct {
	provRepo *persistence.ProviderRepo
	registry *providers.Registry
	logger   *slog.Logger
}

// NewModelsHandler returns a new ModelsHandler.
func NewModelsHandler(
	pr *persistence.ProviderRepo,
	reg *providers.Registry,
	logger *slog.Logger,
) *ModelsHandler {
	return &ModelsHandler{provRepo: pr, registry: reg, logger: logger}
}

// Handle responds with an aggregated list of models from all enabled providers.
func (h *ModelsHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	enabledProviders, err := h.provRepo.ListEnabled(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "models: list providers", slog.String("error", err.Error()))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list providers")
	}

	var allModels []domain.Model

	for _, p := range enabledProviders {
		// Build adapter with an empty key for ListModels (no auth needed for static lists).
		// For providers needing auth (OpenAI, OpenRouter), we skip on error.
		adapter, err := h.registry.Build(&p, "")
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
