package crypto_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/tnotstar/go-ya/internal/crypto"
)

// generateKey produces a 32-byte random master key for tests.
func generateKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("sk-supersecretapikey1234567890")

	ciphertext, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Ciphertext must differ from plaintext.
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("Encrypt() returned plaintext unchanged")
	}

	got, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt() got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("same-plaintext")

	ct1, _ := crypto.Encrypt(plaintext, key)
	ct2, _ := crypto.Encrypt(plaintext, key)

	// Two encryptions of the same plaintext must produce different ciphertexts
	// because each uses a random 12-byte nonce.
	if bytes.Equal(ct1, ct2) {
		t.Error("Encrypt() produced identical ciphertexts for identical plaintexts (nonce reuse)")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := generateKey(t)
	key2 := generateKey(t)
	plaintext := []byte("secret")

	ct, _ := crypto.Encrypt(plaintext, key1)
	_, err := crypto.Decrypt(ct, key2)
	if err == nil {
		t.Error("Decrypt() with wrong key should return an error")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := generateKey(t)
	_, err := crypto.Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("Decrypt() with short ciphertext should return an error")
	}
}

func TestHashKeyConsistency(t *testing.T) {
	h1 := crypto.HashKey("my-api-key")
	h2 := crypto.HashKey("my-api-key")
	if h1 != h2 {
		t.Errorf("HashKey() is not deterministic: %q != %q", h1, h2)
	}
}

func TestHashKeyDifferentInputs(t *testing.T) {
	h1 := crypto.HashKey("key-a")
	h2 := crypto.HashKey("key-b")
	if h1 == h2 {
		t.Error("HashKey() produced identical hashes for different inputs")
	}
}
