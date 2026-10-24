package db

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

type encryptedTestDevice struct {
	user    int
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
	n       int
}

func newEncryptedTestDevice(t *testing.T, s *Store, user int, n int) *encryptedTestDevice {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	chatMust(t, err)
	d := &encryptedTestDevice{user: user, id: chatNonce(n), public: pub, private: priv}
	out, err := d.call(t, s, "register", map[string]any{"name": "Test browser", "public_key": pub})
	chatMust(t, err)
	if out.Device == nil {
		t.Fatal("device not registered")
	}
	return d
}
func (d *encryptedTestDevice) request(t *testing.T, op string, p any) EncryptedRequest {
	t.Helper()
	d.n++
	raw, err := json.Marshal(p)
	chatMust(t, err)
	r := EncryptedRequest{Op: op, DeviceID: d.id, Nonce: chatNonce(d.n), Payload: string(raw)}
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(d.private, []byte(EncryptedSigningText(d.user, r))))
	return r
}
func (d *encryptedTestDevice) call(t *testing.T, s *Store, op string, p any) (EncryptedResponse, error) {
	t.Helper()
	return s.EncryptedChat(t.Context(), d.user, d.request(t, op, p))
}
func encryptedPublish(t *testing.T, s *Store, d *encryptedTestDevice, n int) {
	t.Helper()
	_, err := d.call(t, s, "packages", map[string]any{"packages": []ChatPackage{{ID: chatNonce(n), Package: []byte(strings.Repeat("public key package", 8))}}})
	chatMust(t, err)
}
func encryptedStart(t *testing.T, s *Store, d *encryptedTestDevice, target ChatTarget) int {
	t.Helper()
	out, err := d.call(t, s, "start", map[string]any{"target": target})
	chatMust(t, err)
	return out.ConversationID
}
func encryptedNewSession(t *testing.T, s *Store, d *encryptedTestDevice, id, n int) EncryptedSession {
	t.Helper()
	out, err := d.call(t, s, "roster", map[string]any{"conversation_id": id})
	chatMust(t, err)
	claim, err := d.call(t, s, "claim", map[string]any{"conversation_id": id, "roster_hash": out.Roster.Hash})
	chatMust(t, err)
	ids := []string{}
	for _, p := range claim.Packages {
		ids = append(ids, p.ID)
	}
	out, err = d.call(t, s, "session", map[string]any{"conversation_id": id, "roster_hash": out.Roster.Hash, "session_id": chatNonce(n), "packages": ids, "welcome": []byte(strings.Repeat("opaque welcome", 8))})
	chatMust(t, err)
	return *out.Session
}
func encryptedSendInput(id int, session EncryptedSession, after int) map[string]any {
	return map[string]any{"conversation_id": id, "session_id": session.ID, "roster_hash": session.RosterHash, "kind": "message", "after": after, "ciphertext": []byte(strings.Repeat("opaque ciphertext", 4))}
}
func TestEncryptedDevicesRequireTrustedApprovalAndSignatures(t *testing.T) {
	s := newTestStore(t)
	user, other, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin)
	a := newEncryptedTestDevice(t, s, user, 100)
	b := newEncryptedTestDevice(t, s, user, 101)
	_, err := b.call(t, s, "packages", map[string]any{"packages": []ChatPackage{}})
	chatWantError(t, err, ErrChatDeviceApproval)
	r := a.request(t, "devices", map[string]any{})
	r.Payload = `{"tampered":true}`
	_, err = s.EncryptedChat(t.Context(), user, r)
	chatWantError(t, err, ErrChatNotFound)
	r = a.request(t, "devices", map[string]any{})
	_, err = s.EncryptedChat(t.Context(), other, r)
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.EncryptedChat(t.Context(), admin, r)
	chatWantError(t, err, ErrChatNotFound)
	_, err = a.call(t, s, "approve", map[string]any{"id": b.id, "public_key": b.public})
	chatMust(t, err)
	encryptedPublish(t, s, b, 102)
	_, err = a.call(t, s, "revoke", map[string]any{"id": b.id, "public_key": b.public})
	chatMust(t, err)
	_, err = b.call(t, s, "devices", map[string]any{})
	chatWantError(t, err, ErrChatNotFound)
	_, err = a.call(t, s, "revoke", map[string]any{"id": a.id, "public_key": a.public})
	chatMust(t, err)
	c := newEncryptedTestDevice(t, s, user, 103)
	_, err = c.call(t, s, "start", map[string]any{"target": ChatTarget{Handle: chatHandle(t, s, other)}})
	chatWantError(t, err, ErrChatDeviceApproval)
}
func TestEncryptedMessagesOrderingMembershipAndDisclosures(t *testing.T) {
	s := newTestStore(t)
	buyer, peer, outsider, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin)
	a, b, x := newEncryptedTestDevice(t, s, buyer, 110), newEncryptedTestDevice(t, s, peer, 111), newEncryptedTestDevice(t, s, outsider, 112)
	encryptedPublish(t, s, b, 113)
	id := encryptedStart(t, s, a, ChatTarget{Handle: "@" + chatHandle(t, s, peer)})
	session := encryptedNewSession(t, s, a, id, 114)
	input := encryptedSendInput(id, session, 0)
	request := a.request(t, "send", input)
	first, err := s.EncryptedChat(t.Context(), buyer, request)
	chatMust(t, err)
	duplicate, err := s.EncryptedChat(t.Context(), buyer, request)
	chatMust(t, err)
	if duplicate.ID != first.ID {
		t.Fatal("duplicate encrypted event")
	}
	_, err = a.call(t, s, "send", encryptedSendInput(id, session, first.ID))
	chatWantError(t, err, ErrChatClosed)
	chatMust(t, s.ChatAction(t.Context(), peer, id, "accept", 0, ""))
	_, err = b.call(t, s, "send", encryptedSendInput(id, session, 0))
	chatWantError(t, err, ErrChatEventOrder)
	second, err := b.call(t, s, "send", encryptedSendInput(id, session, first.ID))
	chatMust(t, err)
	for _, op := range []string{"roster", "events"} {
		_, err = x.call(t, s, op, map[string]any{"conversation_id": id})
		chatWantError(t, err, ErrChatNotFound)
	}
	events, err := b.call(t, s, "events", map[string]any{"conversation_id": id})
	chatMust(t, err)
	if len(events.Events) != 2 || len(events.Sessions) != 1 {
		t.Fatalf("events=%+v", events)
	}
	for _, disclosed := range []string{" \n\t ", "text\x00here", "text\x1b[31mhere", "text\x7fhere", strings.Repeat("a", 4001)} {
		_, err = b.call(t, s, "report", map[string]any{"conversation_id": id, "event_id": first.ID, "reason": "spam", "disclosed_text": disclosed})
		chatWantError(t, err, ErrChatInvalid)
	}
	var reportCount int
	chatMust(t, s.DB.QueryRow(`SELECT count(*) FROM chat_encrypted_reports`).Scan(&reportCount))
	if reportCount != 0 {
		t.Fatal("invalid disclosure was persisted")
	}
	const disclosure = "  selected recipient disclosure\r\n\t<script>quoted text</script>  "
	_, err = b.call(t, s, "report", map[string]any{"conversation_id": id, "event_id": first.ID, "reason": "spam", "disclosed_text": disclosure})
	chatMust(t, err)
	reports, err := s.ChatReports(t.Context(), admin)
	chatMust(t, err)
	if len(reports) != 1 || !reports[0].Encrypted || reports[0].Body != disclosure {
		t.Fatalf("reports=%+v", reports)
	}
	chatMust(t, s.ResolveChatReport(t.Context(), admin, -reports[0].ID))
	_, err = b.call(t, s, "read", map[string]any{"conversation_id": id, "event_id": second.ID})
	chatMust(t, err)
	pending := newEncryptedTestDevice(t, s, peer, 115)
	_, err = b.call(t, s, "approve", map[string]any{"id": pending.id, "public_key": pending.public})
	chatMust(t, err)
	events, err = pending.call(t, s, "events", map[string]any{"conversation_id": id})
	chatMust(t, err)
	if len(events.Events) != 0 {
		t.Fatal("new device inherited past history")
	}
	_, err = a.call(t, s, "send", encryptedSendInput(id, session, second.ID))
	chatWantError(t, err, ErrChatRosterChanged)
	roster, err := pending.call(t, s, "roster", map[string]any{"conversation_id": id})
	chatMust(t, err)
	_, err = pending.call(t, s, "claim", map[string]any{"conversation_id": id, "roster_hash": roster.Roster.Hash})
	chatWantError(t, err, ErrChatDeviceApproval)
}
func TestEncryptedCourierRevokesCachedResponses(t *testing.T) {
	s := newTestStore(t)
	buyer, c1, c2, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleCourier), mustCreateUser(t, s, RoleCourier), mustCreateUser(t, s, RoleAdmin)
	_, _, product := chatShop(t, s)
	order := chatOrder(t, s, buyer, product)
	_, err := s.DB.Exec(`UPDATE orders SET status='shipped',delivery_status='shipped',partner_status='dispatched',inventory_state='shipped' WHERE id=$1`, order)
	chatMust(t, err)
	chatMust(t, s.AssignCourierToOrder(t.Context(), order, c1, admin))
	a, b := newEncryptedTestDevice(t, s, buyer, 120), newEncryptedTestDevice(t, s, c1, 121)
	encryptedPublish(t, s, b, 122)
	id := encryptedStart(t, s, a, ChatTarget{DeliveryOrderID: order})
	session := encryptedNewSession(t, s, a, id, 123)
	request := b.request(t, "send", encryptedSendInput(id, session, 0))
	_, err = s.EncryptedChat(t.Context(), c1, request)
	chatMust(t, err)
	chatMust(t, s.AssignCourierToOrder(t.Context(), order, c2, admin))
	_, err = s.EncryptedChat(t.Context(), c1, request)
	chatWantError(t, err, ErrChatNotFound)
	_, err = b.call(t, s, "events", map[string]any{"conversation_id": id})
	chatWantError(t, err, ErrChatNotFound)
}

func TestEncryptedExpiredPublicPackagesAreNotClaimed(t *testing.T) {
	s := newTestStore(t)
	a := newEncryptedTestDevice(t, s, mustCreateUser(t, s, RoleUser), 130)
	b := newEncryptedTestDevice(t, s, mustCreateUser(t, s, RoleUser), 131)
	encryptedPublish(t, s, b, 132)
	_, err := s.DB.Exec(`UPDATE chat_device_packages SET created_at=now()-interval '31 days' WHERE device_id=$1`, b.id)
	chatMust(t, err)
	available, err := b.call(t, s, "devices", map[string]any{})
	chatMust(t, err)
	if available.PackagesAvailable != 0 {
		t.Fatal("stale packages counted as usable")
	}
	id := encryptedStart(t, s, a, ChatTarget{Handle: chatHandle(t, s, b.user)})
	roster, err := a.call(t, s, "roster", map[string]any{"conversation_id": id})
	chatMust(t, err)
	_, err = a.call(t, s, "claim", map[string]any{"conversation_id": id, "roster_hash": roster.Roster.Hash})
	chatWantError(t, err, ErrChatDeviceApproval)
	encryptedPublish(t, s, b, 133)
	claim, err := a.call(t, s, "claim", map[string]any{"conversation_id": id, "roster_hash": roster.Roster.Hash})
	chatMust(t, err)
	if len(claim.Packages) != 1 || claim.Packages[0].ID != chatNonce(133) {
		t.Fatalf("claimed stale key package: %+v", claim.Packages)
	}
}

func TestEncryptedDisclosureTextValidation(t *testing.T) {
	for _, text := range []string{"hello", "  exact excerpt  ", "line one\r\n\tline two\n", strings.Repeat("😀", 4000)} {
		if !validEncryptedDisclosure(text) {
			t.Fatalf("valid excerpt rejected (%d bytes)", len(text))
		}
	}
	for _, text := range []string{"", " \n\r\n\t", "a\x00b", "a\x1bb", "a\rb", "bad\xfftext", strings.Repeat("😀", 4001)} {
		if validEncryptedDisclosure(text) {
			t.Fatalf("invalid excerpt accepted (%d bytes)", len(text))
		}
	}
}
