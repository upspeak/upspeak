// Package secrets provides the AES-GCM implementation of core.SecretCipher used
// to encrypt connection credentials at rest. The master key is sourced from the
// environment; cryptography is kept out of the core domain package.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/upspeak/upspeak/core"
)

// EnvKeyName is the environment variable holding the base64-encoded 32-byte
// master key.
const EnvKeyName = "UPSPEAK_SECRET_KEY"

// Cipher is an AES-256-GCM implementation of core.SecretCipher.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher creates a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromEnv builds a Cipher from the base64-encoded key in UPSPEAK_SECRET_KEY.
// Returns (nil, nil) when the variable is unset, so callers can run without
// credential encryption until a connection actually needs it (fail-closed at use).
func NewCipherFromEnv() (*Cipher, error) {
	raw := os.Getenv(EnvKeyName)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded: %w", EnvKeyName, err)
	}
	return NewCipher(key)
}

// Encrypt seals plaintext with a fresh random nonce, returning nonce||ciphertext.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens nonce||ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, body := ciphertext[:ns], ciphertext[ns:]
	plain, err := c.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return plain, nil
}

// Ensure Cipher satisfies the core port.
var _ core.SecretCipher = (*Cipher)(nil)
