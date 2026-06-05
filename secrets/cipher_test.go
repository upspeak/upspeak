package secrets

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestCipher_RoundTrip(t *testing.T) {
	c, err := NewCipher(testKey())
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	plain := []byte(`{"api_key":"secret-123"}`)

	ct, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ct, []byte("secret-123")) {
		t.Fatal("ciphertext leaks plaintext")
	}

	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestCipher_NonceIsRandom(t *testing.T) {
	c, _ := NewCipher(testKey())
	plain := []byte("same input")
	a, _ := c.Encrypt(plain)
	b, _ := c.Encrypt(plain)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same plaintext are identical; nonce not random")
	}
}

func TestCipher_TamperDetected(t *testing.T) {
	c, _ := NewCipher(testKey())
	ct, _ := c.Encrypt([]byte("data"))
	ct[len(ct)-1] ^= 0xFF // flip a byte
	if _, err := c.Decrypt(ct); err == nil {
		t.Fatal("expected GCM authentication failure on tampered ciphertext")
	}
}

func TestNewCipher_BadKeyLength(t *testing.T) {
	if _, err := NewCipher([]byte("too-short")); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
