package core

import "errors"

// ErrSecretKeyMissing indicates that credential encryption was requested but no
// master key is configured. Operations that must read or write credentials fail
// closed with this error rather than handling secrets in plaintext.
var ErrSecretKeyMissing = errors.New("secret encryption key is not configured")

// SecretCipher encrypts and decrypts credential material at rest. The
// implementation lives outside core (see the secrets package) and is injected
// into the archive, keeping cryptography out of the domain layer.
type SecretCipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
