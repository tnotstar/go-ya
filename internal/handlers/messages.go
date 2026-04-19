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
	"github.com/tnotstar/go-ya/internal/translators"
)

// MessagesHandler handles POST /v1/messages (Anthropic Messages API format).
type MessagesHandler struct {
	router  *services.RouterService
	session *services.SessionService
	logger  *slog.Logger
}

// NewMessagesHandler returns a new MessagesHandler.
func NewMessagesHandler(r *services.RouterService, s *services.SessionService, logger *slog.Logger) *MessagesHandler {
	return &MessagesHandler{router: r, session: s, logger: logger}
}

// Handle processes an Anthropic Messages API request.
func (h *MessagesHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	var anthropicReq translators.AnthropicRequest
	if err := c.Bind(&anthropicReq); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if anthropicReq.Model == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "model is required")
	}

	canonical, err := translators.AnthropicToCanonical(&anthropicReq)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "request translation error: "+err.Error())
	}

	canonical.SessionID = c.Request().Header.Get("X-Session-ID")
	canonical.UserID = userIDFromCtx(c)
	canonical.ClientType = domain.ClientTypeClaudeCode
	canonical.OriginalModel = canonical.Model

	sess, err := h.session.GetOrCreate(ctx, canonical.SessionID, canonical.UserID, canonical.ClientType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session error")
	}
	canonical.SessionID = sess.ID
	c.Response().Header().Set("X-Session-ID", sess.ID)

	if canonical.Stream {
		return h.handleStream(c, canonical)
	}
	return h.handleNonStream(c, canonical)
}

func (h *MessagesHandler) handleNonStream(c echo.Context, req *domain.ChatCompletionRequest) error {
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

	anthropicResp := translators.CanonicalToAnthropic(resp)
	return c.JSON(http.StatusOK, anthropicResp)
}

func (h *MessagesHandler) handleStream(c echo.Context, req *domain.ChatCompletionRequest) error {
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

	state := &translators.AnthropicStreamState{}
	var assembled string

	for event := range ch {
		if event.Err != nil {
			h.logger.ErrorContext(ctx, "messages: stream error", slog.String("error", event.Err.Error()))
			break
		}
		if event.Chunk == nil {
			continue
		}
		if len(event.Chunk.Choices) > 0 {
			assembled += event.Chunk.Choices[0].Delta.Content
		}
		anthropicEvents := translators.CanonicalChunkToAnthropicEvents(event.Chunk, state)
		for _, ae := range anthropicEvents {
			data, err := json.Marshal(ae)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ae.Type, data); err != nil {
				break
			}
		}
		flusher.Flush()
	}

	go h.persistStreamedMessages(req, assembled)
	return nil
}

func (h *MessagesHandler) persistMessages(req *domain.ChatCompletionRequest, resp *domain.ChatCompletionResponse) {
	msgs := services.BuildDomainMessages(req.SessionID, req.Messages)
	if resp != nil && len(resp.Choices) > 0 {
		respMsgs := services.BuildDomainMessages(req.SessionID, []domain.ChatMessage{resp.Choices[0].Message})
		msgs = append(msgs, respMsgs...)
	}
	if err := h.session.AppendMessages(bgCtx(), msgs); err != nil {
		h.logger.Error("messages: persist", slog.String("error", err.Error()))
	}
}

func (h *MessagesHandler) persistStreamedMessages(req *domain.ChatCompletionRequest, assembled string) {
	msgs := services.BuildDomainMessages(req.SessionID, req.Messages)
	content, _ := json.Marshal(assembled)
	msgs = append(msgs, domain.Message{
		SessionID: req.SessionID,
		Role:      domain.RoleAssistant,
		Content:   string(content),
		CreatedAt: time.Now(),
	})
	if err := h.session.AppendMessages(bgCtx(), msgs); err != nil {
		h.logger.Error("messages: persist stream", slog.String("error", err.Error()))
	}
}
