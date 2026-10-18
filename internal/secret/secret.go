// Package secret seals small values, such as payment provider credentials,
// before they are stored in the database. It uses AES-256-GCM with a key from
// the SECRETS_KEY environment variable.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNoKey     = errors.New("SECRETS_KEY is not configured")
	ErrBadKey    = errors.New("SECRETS_KEY must be 32 bytes encoded as base64")
	ErrCorrupted = errors.New("sealed value cannot be opened with this key")
)

// Box seals and opens values. A nil *Box has no key: Seal and Open fail with
// ErrNoKey.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a base64 key of exactly 32 bytes. An empty key
// returns nil, nil.
func New(encodedKey string) (*Box, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(encodedKey)
	}
	if err != nil || len(key) != 32 {
		return nil, ErrBadKey
	}
	return fromKey(key)
}

// Development returns a Box with a fixed, publicly known key. It exists so
// local development works without configuration and must never be used in
// production.
func Development() *Box {
	key := sha256.Sum256([]byte("sokomoko development secrets key - not for production"))
	box, _ := fromKey(key[:])
	return box
}

func fromKey(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The output is nonce followed by ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	if b == nil {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed []byte) ([]byte, error) {
	if b == nil {
		return nil, ErrNoKey
	}
	size := b.aead.NonceSize()
	if len(sealed) < size {
		return nil, ErrCorrupted
	}
	plaintext, err := b.aead.Open(nil, sealed[:size], sealed[size:], nil)
	if err != nil {
		return nil, ErrCorrupted
	}
	return plaintext, nil
}
