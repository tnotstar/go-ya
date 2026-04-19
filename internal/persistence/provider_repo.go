package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/tnotstar/go-ya/internal/domain"
)

// ProviderRepo handles CRUD operations for domain.Provider records.
type ProviderRepo struct {
	db *gorm.DB
}

// NewProviderRepo returns a new ProviderRepo backed by the given DB instance.
func NewProviderRepo(db *gorm.DB) *ProviderRepo {
	return &ProviderRepo{db: db}
}

// Create inserts a new Provider record.
func (r *ProviderRepo) Create(ctx context.Context, p *domain.Provider) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("provider_repo: create: %w", err)
	}
	return nil
}

// ListAll returns all providers ordered by name.
func (r *ProviderRepo) ListAll(ctx context.Context) ([]domain.Provider, error) {
	var providers []domain.Provider
	if err := r.db.WithContext(ctx).Order("name").Find(&providers).Error; err != nil {
		return nil, fmt.Errorf("provider_repo: list: %w", err)
	}
	return providers, nil
}

// ListEnabled returns only enabled providers.
func (r *ProviderRepo) ListEnabled(ctx context.Context) ([]domain.Provider, error) {
	var providers []domain.Provider
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("name").Find(&providers).Error; err != nil {
		return nil, fmt.Errorf("provider_repo: list enabled: %w", err)
	}
	return providers, nil
}

// GetByID returns a provider by its UUID.
func (r *ProviderRepo) GetByID(ctx context.Context, id string) (*domain.Provider, error) {
	var p domain.Provider
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("provider_repo: get by id %q: %w", id, err)
	}
	return &p, nil
}

// GetByName returns a provider by its unique name.
func (r *ProviderRepo) GetByName(ctx context.Context, name string) (*domain.Provider, error) {
	var p domain.Provider
	if err := r.db.WithContext(ctx).First(&p, "name = ?", name).Error; err != nil {
		return nil, fmt.Errorf("provider_repo: get by name %q: %w", name, err)
	}
	return &p, nil
}

// SetEnabled updates the enabled flag for a provider by ID.
func (r *ProviderRepo) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&domain.Provider{}).Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("provider_repo: set enabled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("provider_repo: %w: %s", domain.ErrProviderNotFound, id)
	}
	return nil
}
