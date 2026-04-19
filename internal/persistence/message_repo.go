package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/tnotstar/go-ya/internal/domain"
)

// MessageRepo handles CRUD for domain.Message records.
type MessageRepo struct {
	db *gorm.DB
}

// NewMessageRepo returns a new MessageRepo backed by the given DB.
func NewMessageRepo(db *gorm.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// BatchCreate inserts multiple messages in a single transaction.
// Use this when persisting a full request+response pair atomically.
func (r *MessageRepo) BatchCreate(ctx context.Context, msgs []domain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&msgs).Error
	}); err != nil {
		return fmt.Errorf("message_repo: batch create: %w", err)
	}
	return nil
}

// ListBySession returns all messages for a session ordered by creation time.
func (r *MessageRepo) ListBySession(ctx context.Context, sessionID string) ([]domain.Message, error) {
	var msgs []domain.Message
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("message_repo: list by session: %w", err)
	}
	return msgs, nil
}
