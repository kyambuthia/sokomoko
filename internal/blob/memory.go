package blob

import (
	"context"
	"sync"
)

type memoryItem struct {
	contentType string
	data        []byte
}

// MemoryStore is an in-memory thread-safe implementation of Store.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]memoryItem
}

// NewMemoryStore creates an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]memoryItem),
	}
}

func (m *MemoryStore) Get(_ context.Context, key string) ([]byte, string, error) {
	clean, err := CleanKey(key)
	if err != nil {
		return nil, "", err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[clean]
	if !ok {
		return nil, "", ErrNotFound
	}
	out := make([]byte, len(item.data))
	copy(out, item.data)
	return out, item.contentType, nil
}

func (m *MemoryStore) Put(_ context.Context, key string, contentType string, data []byte) error {
	clean, err := CleanKey(key)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return ErrInvalidPayload
	}

	copied := make([]byte, len(data))
	copy(copied, data)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.items[clean] = memoryItem{
		contentType: contentType,
		data:        copied,
	}
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	clean, err := CleanKey(key)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, clean)
	return nil
}

func (m *MemoryStore) Exists(_ context.Context, key string) (bool, error) {
	clean, err := CleanKey(key)
	if err != nil {
		return false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.items[clean]
	return ok, nil
}
