// Package crypto provides AES-256-GCM encryption and SHA-256 key hashing
// for securing API keys stored in the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

const gcmNonceSize = 12 // standard GCM nonce size in bytes

// ErrInvalidCiphertext is returned when the ciphertext is too short to contain
// a valid nonce prefix.
var ErrInvalidCiphertext = errors.New("crypto: ciphertext too short")

// Encrypt encrypts plaintext using AES-256-GCM with the supplied 32-byte
// master key. The returned byte slice is: [12-byte nonce || ciphertext || tag].
// The master key must not be stored in the database; use the GATEWAY_MASTER_KEY
// environment variable exclusively.
func Encrypt(plaintext, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce := make([]byte, gcmNonceSize)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag after nonce.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts a blob produced by Encrypt using the supplied 32-byte
// master key. It returns the original plaintext or an error.
func Decrypt(ciphertext, masterKey []byte) ([]byte, error) {
	if len(ciphertext) < gcmNonceSize {
		return nil, ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce, sealed := ciphertext[:gcmNonceSize], ciphertext[gcmNonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
