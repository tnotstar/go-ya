package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/tnotstar/go-ya/internal/ctxkeys"
	"github.com/tnotstar/go-ya/internal/domain"
)

// userIDFromCtx reads the user_id stored by auth middleware.
func userIDFromCtx(c echo.Context) string {
	if v := c.Get(string(ctxkeys.UserID)); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "anonymous"
}

// clientTypeFromCtx reads the client_type stored by detector middleware.
func clientTypeFromCtx(c echo.Context) domain.ClientType {
	if v := c.Get(string(ctxkeys.ClientType)); v != nil {
		if ct, ok := v.(domain.ClientType); ok {
			return ct
		}
	}
	return domain.ClientTypeUnknown
}

// mapProviderError converts domain errors into appropriate HTTP error responses.
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, domain.ErrProviderNotFound):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrKeyNotFound):
		return echo.NewHTTPError(http.StatusServiceUnavailable, "no active provider keys")
	case errors.Is(err, domain.ErrRateLimited):
		return echo.NewHTTPError(http.StatusTooManyRequests, "upstream rate limited")
	case errors.Is(err, domain.ErrInvalidKey):
		return echo.NewHTTPError(http.StatusBadGateway, "provider authentication failed")
	case errors.Is(err, domain.ErrProviderOverloaded):
		return echo.NewHTTPError(http.StatusServiceUnavailable, "upstream overloaded")
	default:
		return echo.NewHTTPError(http.StatusBadGateway, "upstream error: "+err.Error())
	}
}

// bgCtx returns a background context for async goroutines that outlive the request.
func bgCtx() context.Context { return context.Background() }
