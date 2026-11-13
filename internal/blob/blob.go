// Package blob provides abstract storage for binary objects such as product
// images and videos, decoupling media blobs from database heap and WAL replication.
package blob

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrNotFound       = errors.New("blob: not found")
	ErrInvalidKey     = errors.New("blob: invalid key")
	ErrInvalidPayload = errors.New("blob: invalid payload")
)

// Store defines operations for storing, retrieving and deleting binary objects.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, string, error)
	Put(ctx context.Context, key string, contentType string, data []byte) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// CleanKey validates and normalizes a storage key, rejecting empty paths
// or path traversal sequences.
func CleanKey(key string) (string, error) {
	clean := strings.TrimSpace(key)
	if clean == "" || len(clean) > 512 {
		return "", ErrInvalidKey
	}
	if strings.Contains(clean, "..") || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "\\") {
		return "", ErrInvalidKey
	}
	return clean, nil
}
