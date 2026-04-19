// Package services contains the application business logic layer.
package services

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tnotstar/go-ya/internal/crypto"
	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
)

// KeyStore manages API key selection, encryption, decryption, and rotation.
// Key selection for a given provider uses a round-robin counter backed by
// an atomic int64 for goroutine safety without a mutex.
type KeyStore struct {
	keyRepo    *persistence.KeyRepo
	masterKey  []byte
	// counters maps providerID → atomic selection counter.
	// A sync.Map is used to allow per-provider counters without a global lock.
	counters   counterMap
}

// counterMap uses a sync.Map to store *atomic.Int64 per provider ID.
type counterMap struct {
	m syncMap
}

func (c *counterMap) next(providerID string, n int) int {
	actual, _ := c.m.LoadOrStore(providerID, new(atomic.Int64))
	counter := actual.(*atomic.Int64)
	idx := int(counter.Add(1) - 1)
	return idx % n
}

// NewKeyStore returns a KeyStore backed by the given key repository.
func NewKeyStore(keyRepo *persistence.KeyRepo, masterKey []byte) *KeyStore {
	return &KeyStore{
		keyRepo:   keyRepo,
		masterKey: masterKey,
	}
}

// GetKey selects the next available enabled key for the provider using round-robin,
// decrypts it, updates last_used_at, and returns the plaintext key.
// Returns domain.ErrKeyNotFound if no enabled keys exist.
func (s *KeyStore) GetKey(ctx context.Context, providerID string) (string, *domain.ProviderKey, error) {
	keys, err := s.keyRepo.ListEnabledByProvider(ctx, providerID)
	if err != nil {
		return "", nil, fmt.Errorf("keystore: list keys: %w", err)
	}
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("%w: provider %s", domain.ErrKeyNotFound, providerID)
	}

	idx := s.counters.next(providerID, len(keys))
	selected := &keys[idx]

	plaintext, err := crypto.Decrypt(selected.EncryptedKey, s.masterKey)
	if err != nil {
		return "", nil, fmt.Errorf("keystore: decrypt: %w", err)
	}

	// Fire-and-forget last_used_at update; non-critical if it fails.
	go func() {
		_ = s.keyRepo.MarkUsed(context.Background(), selected.ID)
	}()

	return string(plaintext), selected, nil
}

// AddKey encrypts the raw key and stores it for the given provider.
// Duplicate detection is done via SHA-256 hash.
func (s *KeyStore) AddKey(ctx context.Context, providerID, label, rawKey string) error {
	hash := crypto.HashKey(rawKey)

	exists, err := s.keyRepo.ExistsByHash(ctx, hash)
	if err != nil {
		return fmt.Errorf("keystore: check duplicate: %w", err)
	}
	if exists {
		return fmt.Errorf("keystore: key already exists for provider %s", providerID)
	}

	encrypted, err := crypto.Encrypt([]byte(rawKey), s.masterKey)
	if err != nil {
		return fmt.Errorf("keystore: encrypt: %w", err)
	}

	k := &domain.ProviderKey{
		ProviderID:   providerID,
		Label:        label,
		EncryptedKey: encrypted,
		KeyHash:      hash,
		Enabled:      true,
	}
	return s.keyRepo.Create(ctx, k)
}

// RotateKey atomically disables the old key and creates a new one.
func (s *KeyStore) RotateKey(ctx context.Context, oldKeyID, label, newRawKey string) error {
	// Retrieve the old key to inherit provider_id.
	oldKey, err := s.keyRepo.GetByID(ctx, oldKeyID)
	if err != nil {
		return fmt.Errorf("keystore: rotate get old key: %w", err)
	}

	hash := crypto.HashKey(newRawKey)
	encrypted, err := crypto.Encrypt([]byte(newRawKey), s.masterKey)
	if err != nil {
		return fmt.Errorf("keystore: rotate encrypt: %w", err)
	}

	newKey := &domain.ProviderKey{
		ProviderID:   oldKey.ProviderID,
		Label:        label,
		EncryptedKey: encrypted,
		KeyHash:      hash,
		Enabled:      true,
	}
	return s.keyRepo.DisableAndCreate(ctx, oldKeyID, newKey)
}

// DisableKey disables an API key by ID.
func (s *KeyStore) DisableKey(ctx context.Context, keyID string) error {
	return s.keyRepo.SetEnabled(ctx, keyID, false)
}

// EnableKey enables an API key by ID.
func (s *KeyStore) EnableKey(ctx context.Context, keyID string) error {
	return s.keyRepo.SetEnabled(ctx, keyID, true)
}
