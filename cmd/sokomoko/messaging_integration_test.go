package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func messageTestSession(t *testing.T, name, role, host string) (int, []*http.Cookie) {
	t.Helper()
	const password = "MessagingFixture123!"
	id := createTestUser(t, name, name+"@example.com", password, role)
	return int(id), []*http.Cookie{loginAndGetSessionCookie(t, host, name, password)}
}
func messagingStatus(t *testing.T, resp *http.Response, body []byte, status int) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, status, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("private response can be cached")
	}
}

func TestMessagingHTTPRequestsPrivacyCSRFAndReporting(t *testing.T) {
	clearAllTables()
	aliceID, alice := messageTestSession(t, "message-alice", db.RoleUser, "")
	bobID, _ := messageTestSession(t, "message-bob", db.RoleUser, "")
	_, outsider := messageTestSession(t, "message-outsider", db.RoleUser, "")
	_, admin := messageTestSession(t, "message-admin", db.RoleAdmin, "admin.localhost")
	target := db.ChatTarget{Handle: "@MESSAGE-BOB"}
	plain := map[string]any{"target": target, "body": "must not be stored", "nonce": fmt.Sprintf("%032x", 1)}
	resp, body := makeAPIRequest(t, "POST", "/api/v1/messages", plain, nil, false, nil)
	messagingStatus(t, resp, body, 401)
	resp, body = makeAPIRequest(t, "POST", "/api/v1/messages", plain, alice, false, nil)
	messagingStatus(t, resp, body, 403)
	resp, body = makeAPIRequest(t, "POST", "/api/v1/messages", plain, alice, true, nil)
	messagingStatus(t, resp, body, 409)
	if !strings.Contains(string(body), "encryption_required") {
		t.Fatal("plaintext fallback allowed")
	}
	page := expectRouteStatus(t, "GET", "/messages/new?handle=%40MESSAGE-BOB", "", nil, alice, 200)
	if !strings.Contains(page, "Open encrypted conversation") || strings.Contains(page, "message-bob@example.com") {
		t.Fatal("encrypted handle lookup")
	}
	// Earlier server-readable history stays explicitly labeled and escaped.
	id, err := testStore.StartChat(t.Context(), aliceID, db.ChatTarget{Handle: "message-bob"}, "<script>alert('x')</script>", fmt.Sprintf("%032x", 2))
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/messages/%d", id)
	page = expectRouteStatus(t, "GET", path+"?legacy=1", "", nil, alice, 200)
	if strings.Contains(page, "<script>alert") || !strings.Contains(page, "&lt;script&gt;") || !strings.Contains(page, "Earlier server-readable history") {
		t.Fatal("legacy disclosure")
	}
	for _, session := range [][]*http.Cookie{outsider, admin} {
		resp, body = makeAPIRequest(t, "GET", "/api/v1"+path, nil, session, false, nil)
		messagingStatus(t, resp, body, 404)
	}
	thread, err := testStore.ChatThread(t.Context(), bobID, id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = testStore.ChatAction(t.Context(), bobID, id, "report", thread.Messages[0].ID, "spam"); err != nil {
		t.Fatal(err)
	}
	page = expectRouteStatus(t, "GET", "/message-reports", "admin.localhost", nil, admin, 200)
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Fatal("legacy report escape")
	}
}

func TestEncryptedMessagingHTTPDeviceSignatureAndCSRF(t *testing.T) {
	clearAllTables()
	user, cookies := messageTestSession(t, "encrypted-user", db.RoleUser, "")
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"name": "HTTP fixture browser", "public_key": pub})
	request := db.EncryptedRequest{Op: "register", DeviceID: fmt.Sprintf("%032x", 100), Nonce: fmt.Sprintf("%032x", 1), Payload: string(payload)}
	request.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, []byte(db.EncryptedSigningText(user, request))))
	resp, body := makeAPIRequest(t, "POST", "/api/v1/encrypted-chat", request, nil, false, nil)
	messagingStatus(t, resp, body, 401)
	resp, body = makeAPIRequest(t, "POST", "/api/v1/encrypted-chat", request, cookies, false, nil)
	messagingStatus(t, resp, body, 403)
	resp, body = makeAPIRequest(t, "POST", "/api/v1/encrypted-chat", request, cookies, true, nil)
	messagingStatus(t, resp, body, 200)
	var enrolled db.EncryptedResponse
	decodeAPIData(t, body, &enrolled)
	if enrolled.Device == nil || !enrolled.Device.Approved {
		t.Fatal("first device not approved")
	}
	request.Op = "devices"
	request.Payload = "{}"
	request.Nonce = fmt.Sprintf("%032x", 2)
	resp, body = makeAPIRequest(t, "POST", "/api/v1/encrypted-chat", request, cookies, true, nil)
	messagingStatus(t, resp, body, 404)
	request.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, []byte(db.EncryptedSigningText(user, request))))
	resp, body = makeAPIRequest(t, "POST", "/api/v1/encrypted-chat", request, cookies, true, nil)
	messagingStatus(t, resp, body, 200)
	if strings.Contains(string(body), "private_key") || strings.Contains(string(body), "@example.com") {
		t.Fatal("device endpoint exposed private data")
	}
}

func TestMessagingHTTPShopAndCourierWorkspaces(t *testing.T) {
	clearAllTables()
	adminID, _ := messageTestSession(t, "message-dispatch", db.RoleAdmin, "admin.localhost")
	buyerID, buyer := messageTestSession(t, "message-buyer", db.RoleUser, "")
	createTestUser(t, "message-shop", "message-shop@example.com", "MessagingFixture123!", db.RolePartner)
	partnerID := createAPITestPartner(t, "message-shop")
	shop := []*http.Cookie{loginAndGetSessionCookie(t, "partner.localhost", "message-shop", "MessagingFixture123!")}
	courierID, courier := messageTestSession(t, "message-courier", db.RoleCourier, "courier.localhost")
	productID := createTestProduct(t, "Chat fixture item", "chat-fixture-item", 1000, 5)
	if _, err := testStore.DB.Exec(`UPDATE products SET partner_id=$1 WHERE id=$2`, partnerID, productID); err != nil {
		t.Fatal(err)
	}
	productPage := expectRouteStatus(t, "GET", "/products/chat-fixture-item", "", nil, buyer, 200)
	if !strings.Contains(productPage, "Message seller") {
		t.Fatal("missing product chat entry")
	}
	id, err := testStore.StartChat(t.Context(), buyerID, db.ChatTarget{ProductID: int(productID)}, "product question", fmt.Sprintf("%032x", 1))
	if err != nil {
		t.Fatal(err)
	}
	page := expectRouteStatus(t, "GET", fmt.Sprintf("/messages/%d", id), "partner.localhost", nil, shop, 200)
	if strings.Contains(page, "product question") || !strings.Contains(page, "@message-shop · End-to-end encrypted messaging") {
		t.Fatal("partner encrypted shell failed")
	}
	if err := testStore.AddToCart(t.Context(), buyerID, int(productID), 1); err != nil {
		t.Fatal(err)
	}
	order, err := testStore.PlaceOrderFromCart(t.Context(), buyerID, "Sample delivery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testStore.DB.Exec(`UPDATE orders SET status='shipped',delivery_status='shipped',partner_status='dispatched',inventory_state='shipped' WHERE id=$1`, order); err != nil {
		t.Fatal(err)
	}
	if err = testStore.AssignCourierToOrder(t.Context(), int(order), courierID, adminID); err != nil {
		t.Fatal(err)
	}
	expectRouteStatus(t, "GET", fmt.Sprintf("/messages/new?order_id=%d", order), "", nil, buyer, 200)
	page = expectRouteStatus(t, "GET", "/deliveries", "courier.localhost", nil, courier, 200)
	if !strings.Contains(page, "Message buyer") {
		t.Fatal("missing courier entry point")
	}
	id, err = testStore.StartChat(t.Context(), courierID, db.ChatTarget{DeliveryOrderID: int(order)}, "arriving shortly", fmt.Sprintf("%032x", 2))
	if err != nil {
		t.Fatal(err)
	}
	expectRouteStatus(t, "GET", fmt.Sprintf("/messages/%d", id), "courier.localhost", nil, courier, 200)
	page = expectRouteStatus(t, "GET", fmt.Sprintf("/messages/%d", id), "", nil, buyer, 200)
	if strings.Contains(page, "arriving shortly") || !strings.Contains(page, "ui-encrypted-chat") {
		t.Fatal("courier plaintext leaked into encrypted shell")
	}
	replacement := int(createTestUser(t, "replacement-rider", "replacement-rider@example.com", "FixturePass123!", db.RoleCourier))
	if err = testStore.AssignCourierToOrder(t.Context(), int(order), replacement, adminID); err != nil {
		t.Fatal(err)
	}
	resp, body := makeAPIRequestOnHost(t, "courier.localhost", "GET", fmt.Sprintf("/api/v1/messages/%d?after=1", id), nil, courier, false, nil)
	messagingStatus(t, resp, body, 404)
	expectRouteStatus(t, "GET", fmt.Sprintf("/messages/%d", id), "courier.localhost", nil, courier, 404)
}
