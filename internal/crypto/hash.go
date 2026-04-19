package crypto

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashKey returns the SHA-256 hex digest of the raw API key string.
// This hash is used for deduplication and lookup without exposing the key.
// It is safe to store in the database.
func HashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
