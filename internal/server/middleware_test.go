package server_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/tnotstar/go-ya/internal/server"
)

// Regression test: Echo writes 404s for unmatched routes only after the
// middleware chain returns, so the log must derive the status from the
// error instead of reporting the unwritten default 200.
func TestSlogMiddleware_LogsRealStatusOnNotFound(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(server.NewSlogMiddleware(logger))

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "status=404") {
		t.Errorf("log should contain status=404, got: %s", out)
	}
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("404 should be logged at level=WARN, got: %s", out)
	}
}

func TestSlogMiddleware_LogsHandlerHTTPErrorStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(server.NewSlogMiddleware(logger))
	e.GET("/fail", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusBadGateway, "upstream down")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if !strings.Contains(buf.String(), "status=502") {
		t.Errorf("log should contain status=502, got: %s", buf.String())
	}
}
