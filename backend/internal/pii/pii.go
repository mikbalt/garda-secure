// Package pii encrypts personal data stored in the database (BE-S9, UU PDP
// 27/2022): visitor ID card numbers are AES-256-GCM encrypted with a key
// derived from SETTINGS_ENCRYPTION_KEY via HKDF-SHA256 (info
// "securepatrol pii v1") and shown masked except to the roles that need them.
package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
)

// Prefix marks an encrypted value ("pii1:" + base64url(nonce | ciphertext)).
const Prefix = "pii1:"

const prefix = Prefix

// info binds the derived key to this purpose and version.
const info = "securepatrol pii v1"

// ErrDecrypt reports a value that is marked encrypted but cannot be opened
// (wrong key or corrupted).
var ErrDecrypt = errors.New("pii: cannot decrypt value")

// Cipher encrypts and decrypts PII values.
type Cipher struct{ aead cipher.AEAD }

// New derives the PII key from the settings secret.
func New(secret string) (*Cipher, error) {
	if secret == "" {
		return nil, errors.New("pii: empty secret")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, info, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

var cache sync.Map // secret -> *Cipher

// For returns a cached cipher for a secret.
func For(secret string) (*Cipher, error) {
	if c, ok := cache.Load(secret); ok {
		return c.(*Cipher), nil
	}
	c, err := New(secret)
	if err != nil {
		return nil, err
	}
	cache.Store(secret, c)
	return c, nil
}

// IsEncrypted reports whether a stored value is already encrypted.
func IsEncrypted(s string) bool { return strings.HasPrefix(s, prefix) }

// Encrypt returns the stored form of a value; empty values and values that
// are already encrypted are returned unchanged.
func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" || IsEncrypted(plain) {
		return plain, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plain), []byte(info))
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt returns the plain value; legacy plaintext (no prefix) is returned
// as is.
func (c *Cipher) Decrypt(stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	n := c.aead.NonceSize()
	if err != nil || len(raw) < n {
		return "", ErrDecrypt
	}
	plain, err := c.aead.Open(nil, raw[:n], raw[n:], []byte(info))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

// Mask hides the middle of an identifier: 3201234567890001 -> 3201********0001.
// Short values keep only their last two characters.
func Mask(s string) string {
	r := []rune(s)
	switch {
	case len(r) == 0:
		return ""
	case len(r) > 8:
		return string(r[:4]) + strings.Repeat("*", len(r)-8) + string(r[len(r)-4:])
	case len(r) > 2:
		return strings.Repeat("*", len(r)-2) + string(r[len(r)-2:])
	}
	return strings.Repeat("*", len(r))
}
