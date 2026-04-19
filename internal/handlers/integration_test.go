package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
	"github.com/tnotstar/go-ya/internal/server"
	"github.com/tnotstar/go-ya/internal/services"
	"github.com/tnotstar/go-ya/internal/translators"
	"gorm.io/gorm/logger"
)

type discardWriter struct{}

func (w discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func setupTestServer(t *testing.T) (*echo.Echo, *persistence.ProviderRepo, *services.KeyStore) {
	t.Helper()

	db, err := persistence.Open("sqlite", ":memory:", logger.Silent)
	if err != nil {
		t.Fatalf("Failed to open test db: %v", err)
	}

	provRepo := persistence.NewProviderRepo(db)
	keyRepo := persistence.NewKeyRepo(db)
	sessionRepo := persistence.NewSessionRepo(db)
	msgRepo := persistence.NewMessageRepo(db)

	masterKey := []byte("01234567890123456789012345678901") // 32 bytes
	keyStore := services.NewKeyStore(keyRepo, masterKey)
	sessionSvc := services.NewSessionService(sessionRepo, msgRepo)

	cfg := &config.Config{
		DefaultProvider: "mock",
		MaxRetries:      3,
	}
	registry := providers.NewRegistry(cfg)
	router := services.NewRouterService(provRepo, keyStore, registry, "mock", 3)

	svc := &server.Services{
		Router:   router,
		Session:  sessionSvc,
		KeyStore: keyStore,
		ProvRepo: provRepo,
		Registry: registry,
	}

	discardLogger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	e := server.New(cfg, svc, discardLogger)
	
	// Create mock provider
	prov := &domain.Provider{
		Name:    "mock",
		Type:    domain.ProviderTypeMock,
		Enabled: true,
	}
	_ = provRepo.Create(context.Background(), prov)
	_ = keyStore.AddKey(context.Background(), prov.ID, "test-key", "sk-mock-123")

	return e, provRepo, keyStore
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHealthHandler(t *testing.T) {
	e, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestChatCompletion_Success(t *testing.T) {
	e, _, _ := setupTestServer(t)

	chatReq := domain.ChatCompletionRequest{
		Model: "mock:test-model",
		Messages: []domain.ChatMessage{
			{Role: "user", Content: mustJSON(t, "Hello")},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(mustJSON(t, chatReq)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. body: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	
	var res domain.ChatCompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if len(res.Choices) == 0 {
		t.Fatalf("expected choices, got none")
	}
	
	var content string
	_ = json.Unmarshal(res.Choices[0].Message.Content, &content)
	if content != "Mock response from test provider." {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMessages_AnthropicEndpoint_Success(t *testing.T) {
	e, _, _ := setupTestServer(t)

	anthropicReq := translators.AnthropicRequest{
		Model: "mock:claude-fake",
		MaxTokens: 1024,
		Messages: []translators.AnthropicMessage{
			{Role: "user", Content: translators.AnthropicContent(mustJSON(t, "Hello Claude"))},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(mustJSON(t, anthropicReq)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. body: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	
	var res translators.AnthropicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if res.Type != "message" || res.Role != "assistant" {
		t.Errorf("invalid response role/type: %s / %s", res.Role, res.Type)
	}
	if len(res.Content) == 0 || res.Content[0].Text != "Mock response from test provider." {
		t.Errorf("unexpected content: %+v", res.Content)
	}
}
