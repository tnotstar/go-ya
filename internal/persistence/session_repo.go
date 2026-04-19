package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/tnotstar/go-ya/internal/domain"
)

// SessionRepo handles CRUD for domain.Session records.
type SessionRepo struct {
	db *gorm.DB
}

// NewSessionRepo returns a new SessionRepo backed by the given DB.
func NewSessionRepo(db *gorm.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

// Create inserts a new Session.
func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("session_repo: create: %w", err)
	}
	return nil
}

// GetByID retrieves a session by UUID.
func (r *SessionRepo) GetByID(ctx context.Context, id string) (*domain.Session, error) {
	var s domain.Session
	if err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("session_repo: get by id %q: %w", id, err)
	}
	return &s, nil
}

// ListByUser returns sessions for a given user ordered by newest first.
func (r *SessionRepo) ListByUser(ctx context.Context, userID string, limit int) ([]domain.Session, error) {
	var sessions []domain.Session
	q := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("session_repo: list by user: %w", err)
	}
	return sessions, nil
}

// UpdateLastProvider sets the last_provider and last_model fields.
func (r *SessionRepo) UpdateLastProvider(ctx context.Context, id, provider, model string) error {
	if err := r.db.WithContext(ctx).Model(&domain.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"last_provider": provider, "last_model": model}).Error; err != nil {
		return fmt.Errorf("session_repo: update last provider: %w", err)
	}
	return nil
}
