// Package handlers implements the Echo HTTP request handlers.
package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Health handles GET /health returning a simple liveness response.
func Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
