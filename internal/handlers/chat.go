package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/providers"
	"github.com/tnotstar/go-ya/internal/services"
)

// ChatHandler handles POST /v1/chat/completions (OpenAI-compatible endpoint).
type ChatHandler struct {
	router  *services.RouterService
	session *services.SessionService
	logger  *slog.Logger
}

// NewChatHandler returns a new ChatHandler.
func NewChatHandler(r *services.RouterService, s *services.SessionService, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{router: r, session: s, logger: logger}
}

// Handle processes a chat completion request (stream or non-stream).
func (h *ChatHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	var req domain.ChatCompletionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Model == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "model is required")
	}

	req.SessionID = c.Request().Header.Get("X-Session-ID")
	req.IdempotencyKey = c.Request().Header.Get("X-Idempotency-Key")
	req.UserID = userIDFromCtx(c)
	req.ClientType = clientTypeFromCtx(c)
	req.OriginalModel = req.Model

	sess, err := h.session.GetOrCreate(ctx, req.SessionID, req.UserID, req.ClientType)
	if err != nil {
		h.logger.ErrorContext(ctx, "chat: session", slog.String("error", err.Error()))
		return echo.NewHTTPError(http.StatusInternalServerError, "session error")
	}
	req.SessionID = sess.ID
	c.Response().Header().Set("X-Session-ID", sess.ID)

	if req.Stream {
		return h.handleStream(c, &req)
	}
	return h.handleNonStream(c, &req)
}

// handleNonStream processes a non-streaming chat completion.
func (h *ChatHandler) handleNonStream(c echo.Context, req *domain.ChatCompletionRequest) error {
	ctx := c.Request().Context()

	var resp *domain.ChatCompletionResponse
	err := h.router.RouteWithFallback(ctx, req.Model, func(rCtx context.Context, res *services.RoutingResult) error {
		reqCopy := *req
		reqCopy.Model = res.Model
		var callErr error
		resp, callErr = res.Provider.ChatCompletion(rCtx, &reqCopy)
		return callErr
	})
	if err != nil {
		return mapProviderError(err)
	}

	go h.persistMessages(req, resp)
	return c.JSON(http.StatusOK, resp)
}

// handleStream processes a streaming chat completion, emitting SSE chunks.
func (h *ChatHandler) handleStream(c echo.Context, req *domain.ChatCompletionRequest) error {
	ctx := c.Request().Context()

	var ch <-chan providers.StreamEvent
	err := h.router.RouteWithFallback(ctx, req.Model, func(rCtx context.Context, res *services.RoutingResult) error {
		reqCopy := *req
		reqCopy.Model = res.Model
		var chanErr error
		ch, chanErr = res.Provider.ChatCompletionStream(rCtx, &reqCopy)
		return chanErr
	})
	if err != nil {
		return mapProviderError(err)
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	var assembled string
	var lastModel string
	for event := range ch {
		if event.Err != nil {
			h.logger.ErrorContext(ctx, "chat: stream error", slog.String("error", event.Err.Error()))
			break
		}
		if event.Chunk == nil {
			continue
		}
		lastModel = event.Chunk.Model
		if len(event.Chunk.Choices) > 0 {
			assembled += event.Chunk.Choices[0].Delta.Content
		}
		data, err := json.Marshal(event.Chunk)
		if err != nil {
			break
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			break
		}
		flusher.Flush()
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()

	go h.persistStreamedMessages(req, assembled, lastModel)
	return nil
}

func (h *ChatHandler) persistMessages(req *domain.ChatCompletionRequest, resp *domain.ChatCompletionResponse) {
	msgs := services.BuildDomainMessages(req.SessionID, req.Messages)
	if resp != nil && len(resp.Choices) > 0 {
		respMsgs := services.BuildDomainMessages(req.SessionID, []domain.ChatMessage{resp.Choices[0].Message})
		msgs = append(msgs, respMsgs...)
	}
	if err := h.session.AppendMessages(bgCtx(), msgs); err != nil {
		h.logger.Error("chat: persist messages", slog.String("error", err.Error()))
	}
}

func (h *ChatHandler) persistStreamedMessages(req *domain.ChatCompletionRequest, assembled, model string) {
	msgs := services.BuildDomainMessages(req.SessionID, req.Messages)
	content, _ := json.Marshal(assembled)
	msgs = append(msgs, domain.Message{
		SessionID: req.SessionID,
		Role:      domain.RoleAssistant,
		Content:   string(content),
		CreatedAt: time.Now(),
	})
	if err := h.session.AppendMessages(bgCtx(), msgs); err != nil {
		h.logger.Error("chat: persist stream messages", slog.String("error", err.Error()))
	}
	_ = h.session.UpdateProvider(bgCtx(), req.SessionID, model, model)
}
