package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
)

// SessionService manages conversation session lifecycle and message persistence.
type SessionService struct {
	sessionRepo *persistence.SessionRepo
	messageRepo *persistence.MessageRepo
}

// NewSessionService returns a new SessionService.
func NewSessionService(sr *persistence.SessionRepo, mr *persistence.MessageRepo) *SessionService {
	return &SessionService{sessionRepo: sr, messageRepo: mr}
}

// GetOrCreate returns an existing session by ID, or creates a new one if the ID
// is empty or not found. The new session ID is returned in the session struct.
func (s *SessionService) GetOrCreate(
	ctx context.Context,
	sessionID, userID string,
	clientType domain.ClientType,
) (*domain.Session, error) {
	if sessionID != "" {
		sess, err := s.sessionRepo.GetByID(ctx, sessionID)
		if err == nil {
			return sess, nil
		}
		// If not found, fall through to create.
	}

	sess := &domain.Session{
		ID:         sessionID, // may be empty; BeforeCreate assigns UUID
		UserID:     userID,
		ClientType: clientType,
	}
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("session: create: %w", err)
	}
	return sess, nil
}

// AppendMessages persists a batch of messages to the session.
// All messages must share the same session_id (the caller's responsibility).
func (s *SessionService) AppendMessages(ctx context.Context, msgs []domain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	if err := s.messageRepo.BatchCreate(ctx, msgs); err != nil {
		return fmt.Errorf("session: append messages: %w", err)
	}
	return nil
}

// UpdateProvider updates the last_provider and last_model fields for a session.
func (s *SessionService) UpdateProvider(ctx context.Context, sessionID, provider, model string) error {
	return s.sessionRepo.UpdateLastProvider(ctx, sessionID, provider, model)
}

// GetMessages returns the full message history for a session.
func (s *SessionService) GetMessages(ctx context.Context, sessionID string) ([]domain.Message, error) {
	return s.messageRepo.ListBySession(ctx, sessionID)
}

// ListSessions returns sessions for a user (most recent first).
func (s *SessionService) ListSessions(ctx context.Context, userID string, limit int) ([]domain.Session, error) {
	return s.sessionRepo.ListByUser(ctx, userID, limit)
}

// GetSession returns a session by ID.
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	return s.sessionRepo.GetByID(ctx, sessionID)
}

// BuildDomainMessages converts canonical ChatMessages from a request into
// domain.Message records suitable for persistence.
func BuildDomainMessages(sessionID string, msgs []domain.ChatMessage) []domain.Message {
	result := make([]domain.Message, 0, len(msgs))
	now := time.Now()
	for _, m := range msgs {
		content := extractStringFromRaw(m.Content)
		var toolCallsRaw domain.JSONRaw
		if len(m.ToolCalls) > 0 {
			if b, err := json.Marshal(m.ToolCalls); err == nil {
				toolCallsRaw = b
			}
		}
		result = append(result, domain.Message{
			SessionID: sessionID,
			Role:      m.Role,
			Content:   content,
			Name:      m.Name,
			ToolCalls: toolCallsRaw,
			CreatedAt: now,
		})
	}
	return result
}

// extractStringFromRaw extracts a plain string from a json.RawMessage.
func extractStringFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
