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
