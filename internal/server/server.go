// Package server configures the Echo HTTP server and registers all routes.
package server

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/handlers"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
	"github.com/tnotstar/go-ya/internal/services"
)

// Services bundles all application services injected into the server.
type Services struct {
	Router   *services.RouterService
	Session  *services.SessionService
	KeyStore *services.KeyStore
	ProvRepo *persistence.ProviderRepo
	Registry *providers.Registry
}

// New creates a fully configured Echo instance with all routes and middleware.
func New(cfg *config.Config, svc *Services, logger *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// ── Middleware stack ────────────────────────────────────────────────────
	e.Use(echomw.RequestID())
	e.Use(echomw.Recover())
	e.Use(NewSlogMiddleware(logger))
	e.Use(NewCORSMiddleware(cfg.CORSOriginList()))
	e.Use(NewClientDetectorMiddleware())

	if len(cfg.AuthTokenMap()) > 0 {
		e.Use(NewAuthMiddleware(cfg.AuthTokenMap()))
	}

	// ── Handler construction ─────────────────────────────────────────────────
	chatHandler := handlers.NewChatHandler(svc.Router, svc.Session, logger)
	messagesHandler := handlers.NewMessagesHandler(svc.Router, svc.Session, logger)
	modelsHandler := handlers.NewModelsHandler(svc.ProvRepo, svc.KeyStore, svc.Registry, logger)

	// ── Routes ───────────────────────────────────────────────────────────────
	e.GET("/health", handlers.Health)

	v1 := e.Group("/v1")
	v1.POST("/chat/completions", chatHandler.Handle)
	v1.POST("/messages", messagesHandler.Handle)
	v1.GET("/models", modelsHandler.Handle)

	// 404 fallback with structured response.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		message := "internal server error"
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			message = he.Message.(string)
		}
		_ = c.JSON(code, map[string]any{
			"error": map[string]any{
				"message": message,
				"type":    "api_error",
				"code":    code,
			},
		})
	}

	return e
}
