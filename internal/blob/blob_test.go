package blob

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testStoreContract(t *testing.T, newStore func(t *testing.T) Store) {
	ctx := context.Background()

	t.Run("put get exists delete lifecycle", func(t *testing.T) {
		s := newStore(t)
		key := "products/42/image.png"
		data := []byte("\x89PNG\r\n\x1a\nfakeimagebytes")
		ctype := "image/png"

		exists, err := s.Exists(ctx, key)
		if err != nil || exists {
			t.Fatalf("expected exists=false, err=nil; got exists=%v err=%v", exists, err)
		}

		if err := s.Put(ctx, key, ctype, data); err != nil {
			t.Fatalf("Put error: %v", err)
		}

		exists, err = s.Exists(ctx, key)
		if err != nil || !exists {
			t.Fatalf("expected exists=true, err=nil; got exists=%v err=%v", exists, err)
		}

		gotData, gotCtype, err := s.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get error: %v", err)
		}
		if gotCtype != ctype {
			t.Fatalf("expected content-type %q, got %q", ctype, gotCtype)
		}
		if !bytes.Equal(gotData, data) {
			t.Fatalf("data mismatch: expected %q, got %q", data, gotData)
		}

		if err := s.Delete(ctx, key); err != nil {
			t.Fatalf("Delete error: %v", err)
		}

		exists, err = s.Exists(ctx, key)
		if err != nil || exists {
			t.Fatalf("expected exists=false after delete, got %v", exists)
		}

		_, _, err = s.Get(ctx, key)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}

		// Delete non-existent key is idempotent
		if err := s.Delete(ctx, key); err != nil {
			t.Fatalf("subsequent delete must succeed, got %v", err)
		}
	})

	t.Run("empty payload rejected", func(t *testing.T) {
		s := newStore(t)
		if err := s.Put(ctx, "empty.bin", "text/plain", []byte{}); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected ErrInvalidPayload, got %v", err)
		}
	})

	t.Run("invalid keys and path traversal rejected", func(t *testing.T) {
		s := newStore(t)
		invalidKeys := []string{
			"",
			"   ",
			"../evil.png",
			"products/../../etc/passwd",
			"/absolute/path.png",
			"\\windows\\path.png",
		}
		for _, k := range invalidKeys {
			if _, _, err := s.Get(ctx, k); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Get(%q) expected ErrInvalidKey, got %v", k, err)
			}
			if err := s.Put(ctx, k, "text/plain", []byte("xyz")); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Put(%q) expected ErrInvalidKey, got %v", k, err)
			}
			if err := s.Delete(ctx, k); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Delete(%q) expected ErrInvalidKey, got %v", k, err)
			}
			if _, err := s.Exists(ctx, k); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Exists(%q) expected ErrInvalidKey, got %v", k, err)
			}
		}
	})
}

func TestMemoryStore(t *testing.T) {
	testStoreContract(t, func(t *testing.T) Store {
		return NewMemoryStore()
	})
}

func TestDiskStore(t *testing.T) {
	testStoreContract(t, func(t *testing.T) Store {
		dir := t.TempDir()
		store, err := NewDiskStore(dir)
		if err != nil {
			t.Fatalf("NewDiskStore error: %v", err)
		}
		return store
	})

	t.Run("handles non-existent base directory creation", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "nested", "storage")
		store, err := NewDiskStore(base)
		if err != nil {
			t.Fatalf("NewDiskStore failed to create nested dirs: %v", err)
		}
		if _, err := os.Stat(base); err != nil {
			t.Fatalf("expected base dir to exist: %v", err)
		}
		_ = store
	})
}
