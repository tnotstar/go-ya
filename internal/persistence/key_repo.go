package persistence

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/tnotstar/go-ya/internal/domain"
)

// KeyRepo handles CRUD operations for domain.ProviderKey records.
type KeyRepo struct {
	db *gorm.DB
}

// NewKeyRepo returns a new KeyRepo backed by the given DB instance.
func NewKeyRepo(db *gorm.DB) *KeyRepo {
	return &KeyRepo{db: db}
}

// Create inserts a new ProviderKey record.
func (r *KeyRepo) Create(ctx context.Context, k *domain.ProviderKey) error {
	if err := r.db.WithContext(ctx).Create(k).Error; err != nil {
		return fmt.Errorf("key_repo: create: %w", err)
	}
	return nil
}

// ListByProvider returns all keys for a given provider ID, ordered by creation time.
func (r *KeyRepo) ListByProvider(ctx context.Context, providerID string) ([]domain.ProviderKey, error) {
	var keys []domain.ProviderKey
	if err := r.db.WithContext(ctx).
		Where("provider_id = ?", providerID).
		Order("created_at").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("key_repo: list by provider: %w", err)
	}
	return keys, nil
}

// ListEnabledByProvider returns active keys for a provider, ordered oldest-first
// (used by round-robin selection in KeyStore).
func (r *KeyRepo) ListEnabledByProvider(ctx context.Context, providerID string) ([]domain.ProviderKey, error) {
	var keys []domain.ProviderKey
	if err := r.db.WithContext(ctx).
		Where("provider_id = ? AND enabled = ?", providerID, true).
		Order("created_at").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("key_repo: list enabled by provider: %w", err)
	}
	return keys, nil
}

// GetByID retrieves a single key by its UUID.
func (r *KeyRepo) GetByID(ctx context.Context, id string) (*domain.ProviderKey, error) {
	var k domain.ProviderKey
	if err := r.db.WithContext(ctx).First(&k, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("key_repo: get by id %q: %w", id, err)
	}
	return &k, nil
}

// ExistsByHash returns true if a key with the given SHA-256 hash already exists.
func (r *KeyRepo) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.ProviderKey{}).
		Where("key_hash = ?", hash).Count(&count).Error; err != nil {
		return false, fmt.Errorf("key_repo: exists by hash: %w", err)
	}
	return count > 0, nil
}

// SetEnabled updates the enabled flag for a key by ID.
func (r *KeyRepo) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&domain.ProviderKey{}).
		Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("key_repo: set enabled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("key_repo: %w: %s", domain.ErrKeyNotFound, id)
	}
	return nil
}

// MarkUsed updates the last_used_at timestamp for a key.
func (r *KeyRepo) MarkUsed(ctx context.Context, id string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&domain.ProviderKey{}).
		Where("id = ?", id).Update("last_used_at", now).Error; err != nil {
		return fmt.Errorf("key_repo: mark used: %w", err)
	}
	return nil
}

// DisableAndCreate disables the key with oldID and creates newKey in a single
// transaction, implementing atomic key rotation.
func (r *KeyRepo) DisableAndCreate(ctx context.Context, oldID string, newKey *domain.ProviderKey) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.ProviderKey{}).
			Where("id = ?", oldID).Update("enabled", false).Error; err != nil {
			return fmt.Errorf("key_repo: rotate disable old: %w", err)
		}
		if err := tx.Create(newKey).Error; err != nil {
			return fmt.Errorf("key_repo: rotate create new: %w", err)
		}
		return nil
	})
}
