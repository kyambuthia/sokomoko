package messaging

import (
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestNormalizeMessagingInputs(t *testing.T) {
	target, err := normalizeTarget(db.ChatTarget{Handle: " @Some.User "})
	if err != nil || target.Handle != "Some.User" {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	for _, target := range []db.ChatTarget{{PartnerID: 1}, {OrderID: 4, PartnerID: 2}} {
		if _, err := normalizeTarget(target); err != nil {
			t.Fatalf("target %+v: %v", target, err)
		}
	}
	for _, target := range []db.ChatTarget{{}, {Handle: "ab"}, {Handle: "foo@example.com"}, {Handle: "valid", ProductID: 1}, {ServiceID: -1}, {ServiceID: 1, ProductID: 2}, {ServiceID: 1, Handle: "valid"}, {ServiceID: 1, PartnerID: 2}, {ProductID: 1, PartnerID: 2}, {OrderID: -1}, {OrderID: 1, DeliveryOrderID: 2}} {
		if _, err := normalizeTarget(target); err == nil {
			t.Fatalf("accepted invalid target %+v", target)
		}
	}
	nonce, err := Nonce()
	if err != nil {
		t.Fatal(err)
	}
	valid := "Hello <script>alert(1)</script>\r\nمرحبا 👋"
	if body, err := normalizeMessage(valid, nonce); err != nil || !strings.Contains(body, "<script>") {
		t.Fatalf("plain text should be preserved for contextual escaping: %q %v", body, err)
	}
	for _, body := range []string{"", "   ", "bad\x00text", string([]byte{0xff}), strings.Repeat("a", 4001)} {
		if _, err := normalizeMessage(body, nonce); err == nil {
			t.Fatal("invalid message accepted")
		}
	}
	if _, err := normalizeMessage("valid", "invalid-nonce"); err == nil {
		t.Fatal("invalid nonce accepted")
	}
}
