package secret

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := New(testKey(t))
	if err != nil || box == nil {
		t.Fatalf("New: %v", err)
	}
	sealed, err := box.Seal([]byte(`{"secret_key":"sk_test_123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("sk_test_123")) {
		t.Fatal("sealed value leaks plaintext")
	}
	opened, err := box.Open(sealed)
	if err != nil || string(opened) != `{"secret_key":"sk_test_123"}` {
		t.Fatalf("Open = %q, %v", opened, err)
	}
	other, _ := New(testKey(t))
	if _, err := other.Open(sealed); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("wrong key Open = %v, want ErrCorrupted", err)
	}
}

func TestNewValidatesKey(t *testing.T) {
	if box, err := New(""); box != nil || err != nil {
		t.Fatalf("empty key = %v, %v", box, err)
	}
	if _, err := New("c2hvcnQ="); !errors.Is(err, ErrBadKey) {
		t.Fatalf("short key err = %v", err)
	}
	var none *Box
	if _, err := none.Seal([]byte("x")); !errors.Is(err, ErrNoKey) {
		t.Fatalf("nil box Seal = %v", err)
	}
	if Development() == nil {
		t.Fatal("development box missing")
	}
}

func TestKeyRotationOpensPreviousAndSealsWithCurrent(t *testing.T) {
	oldKey, newKey := testKey(t), testKey(t)
	old, _ := New(oldKey)
	oldCiphertext, err := old.Seal([]byte("previous credential"))
	if err != nil {
		t.Fatal(err)
	}
	rotating, err := NewWithPrevious(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := rotating.Open(oldCiphertext); err != nil || string(value) != "previous credential" {
		t.Fatalf("open previous = %q, %v", value, err)
	}
	newCiphertext, err := rotating.Seal([]byte("new credential"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Open(newCiphertext); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("old key opened new ciphertext: %v", err)
	}
	current, _ := New(newKey)
	if value, err := current.Open(newCiphertext); err != nil || string(value) != "new credential" {
		t.Fatalf("open new = %q, %v", value, err)
	}
	if _, err := current.Open(oldCiphertext); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("old credential unexpectedly re-encrypted: %v", err)
	}
	if _, err := NewWithPrevious("", oldKey); !errors.Is(err, ErrNoKey) {
		t.Fatalf("previous without current = %v", err)
	}
}
