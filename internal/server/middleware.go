package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/tnotstar/go-ya/internal/ctxkeys"
	"github.com/tnotstar/go-ya/internal/domain"
)

// NewSlogMiddleware returns an Echo middleware that logs each request using slog.
func NewSlogMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			latencyMs := time.Since(start).Milliseconds()

			req := c.Request()
			res := c.Response()

			level := slog.LevelInfo
			if res.Status >= 500 {
				level = slog.LevelError
			} else if res.Status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(req.Context(), level, "request",
				slog.String("request_id", res.Header().Get(echo.HeaderXRequestID)),
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", res.Status),
				slog.Int64("latency_ms", latencyMs),
				slog.String("user_id", userIDFromCtx(c)),
				slog.String("client_type", string(clientTypeFromCtx(c))),
				slog.String("remote_addr", req.RemoteAddr),
			)
			return err
		}
	}
}

// NewCORSMiddleware returns a CORS middleware with the given allowed origins.
func NewCORSMiddleware(origins []string) echo.MiddlewareFunc {
	return echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{
			echo.HeaderAuthorization,
			echo.HeaderContentType,
			"anthropic-version",
			"x-api-key",
			"x-session-id",
			"x-idempotency-key",
		},
		ExposeHeaders: []string{"x-session-id", echo.HeaderXRequestID},
	})
}

// NewAuthMiddleware returns a middleware that validates Bearer tokens.
// tokenMap maps token → userID. If the Authorization header is missing or
// invalid, the request is rejected with HTTP 401.
// If tokenMap is empty, the middleware is a no-op (auth disabled).
func NewAuthMiddleware(tokenMap map[string]string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if len(tokenMap) == 0 {
				return next(c)
			}
			auth := c.Request().Header.Get(echo.HeaderAuthorization)
			token := strings.TrimPrefix(auth, "Bearer ")
			token = strings.TrimSpace(token)

			userID, ok := tokenMap[token]
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or missing API token")
			}
			c.Set(string(ctxkeys.UserID), userID)
			return next(c)
		}
	}
}

// NewClientDetectorMiddleware infers the downstream client type from User-Agent
// and request headers, storing it in the Echo context for logging and session tracking.
func NewClientDetectorMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ct := detectClientType(c.Request())
			c.Set(string(ctxkeys.ClientType), ct)
			return next(c)
		}
	}
}

// detectClientType heuristically identifies the downstream client from headers.
func detectClientType(r *http.Request) domain.ClientType {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case strings.Contains(ua, "librechat"):
		return domain.ClientTypeLibreChat
	case strings.Contains(ua, "claude") || r.Header.Get("anthropic-version") != "":
		return domain.ClientTypeClaudeCode
	case strings.Contains(ua, "opencode"):
		return domain.ClientTypeOpencode
	default:
		return domain.ClientTypeUnknown
	}
}

// userIDFromCtx extracts user_id from Echo context (set by auth middleware).
func userIDFromCtx(c echo.Context) string {
	if v := c.Get(string(ctxkeys.UserID)); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "anonymous"
}

// clientTypeFromCtx extracts client_type from Echo context.
func clientTypeFromCtx(c echo.Context) domain.ClientType {
	if v := c.Get(string(ctxkeys.ClientType)); v != nil {
		if ct, ok := v.(domain.ClientType); ok {
			return ct
		}
	}
	return domain.ClientTypeUnknown
}
