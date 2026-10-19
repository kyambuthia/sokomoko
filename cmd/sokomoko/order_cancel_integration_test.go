package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func availableStock(t *testing.T, productID int64) int {
	t.Helper()
	p, err := testStore.GetProductByID(t.Context(), int(productID))
	if err != nil || p == nil {
		t.Fatalf("load product: %v", err)
	}
	return p.StockQuantity
}

func TestIntegration_CustomerCancelsNewUnpaidOrder(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	buyer := fmt.Sprintf("canceller_%d", suffix)
	buyerID := createTestUser(t, buyer, buyer+"@example.com", "BuyerPass12345", "user")
	productID := createTestProduct(t, "Cancel Mug", fmt.Sprintf("cancel-mug-%d", suffix), 1200, 5)
	if err := testStore.AddToCart(t.Context(), int(buyerID), int(productID), 2); err != nil {
		t.Fatal(err)
	}
	orderID, err := testStore.PlaceOrderFromCart(t.Context(), int(buyerID), "1 Cancel Street")
	if err != nil {
		t.Fatal(err)
	}
	if got := availableStock(t, productID); got != 3 {
		t.Fatalf("available after order = %d", got)
	}
	cookie := loginAndGetSessionCookie(t, "localhost", buyer, "BuyerPass12345")
	cancelPath := fmt.Sprintf("/account/orders/%d/cancel", orderID)

	if _, body := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{cookie}, "localhost"); !strings.Contains(body, cancelPath) {
		t.Fatal("account page offers no cancel action for a new order")
	}

	// Another customer cannot cancel it.
	intruder := fmt.Sprintf("intruder_%d", suffix)
	createTestUser(t, intruder, intruder+"@example.com", "BuyerPass12345", "user")
	other := loginAndGetSessionCookie(t, "localhost", intruder, "BuyerPass12345")
	if resp, _ := makeRequest(http.MethodPost, cancelPath, nil, []*http.Cookie{other}, "localhost"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-customer cancel status=%d", resp.StatusCode)
	}

	resp, _ := makeRequest(http.MethodPost, cancelPath, nil, []*http.Cookie{cookie}, "localhost")
	assertFlashRedirect(t, resp, "/account", fmt.Sprintf("Order #%d cancelled", orderID))
	var status, partnerStatus, inventory, paymentStatus string
	if err := testStore.DB.QueryRow("SELECT o.status, o.partner_status, o.inventory_state, COALESCE(p.status, 'failed') FROM orders o LEFT JOIN payments p ON p.order_id = o.id WHERE o.id = $1", orderID).
		Scan(&status, &partnerStatus, &inventory, &paymentStatus); err != nil {
		t.Fatal(err)
	}
	if status != db.OrderStatusCancelled || partnerStatus != "cancelled" || inventory != "released" || paymentStatus != db.PaymentStatusFailed {
		t.Fatalf("after cancel: status=%s partner=%s inventory=%s payment=%s", status, partnerStatus, inventory, paymentStatus)
	}
	if got := availableStock(t, productID); got != 5 {
		t.Fatalf("available after cancel = %d, want 5", got)
	}
	if _, body := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{cookie}, "localhost"); strings.Contains(body, cancelPath) || !strings.Contains(body, "Cancelled by the customer.") {
		t.Fatal("account page after cancel still offers cancel or hides the notice")
	}
	// Repeating is harmless.
	resp, _ = makeRequest(http.MethodPost, cancelPath, nil, []*http.Cookie{cookie}, "localhost")
	assertFlashRedirect(t, resp, "/account", "cancelled")
	if got := availableStock(t, productID); got != 5 {
		t.Fatalf("available after repeat cancel = %d", got)
	}
}

func TestIntegration_CustomerCannotCancelAcceptedOrder(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	buyer := fmt.Sprintf("late_%d", suffix)
	buyerID := createTestUser(t, buyer, buyer+"@example.com", "BuyerPass12345", "user")
	productID := createTestProduct(t, "Late Mug", fmt.Sprintf("late-mug-%d", suffix), 1200, 5)
	if err := testStore.AddToCart(t.Context(), int(buyerID), int(productID), 1); err != nil {
		t.Fatal(err)
	}
	orderID, err := testStore.PlaceOrderFromCart(t.Context(), int(buyerID), "2 Late Street")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.UpdateOrderFulfillment(t.Context(), int(orderID), "accepted", "processing", ""); err != nil {
		t.Fatal(err)
	}
	cookie := loginAndGetSessionCookie(t, "localhost", buyer, "BuyerPass12345")
	cancelPath := fmt.Sprintf("/account/orders/%d/cancel", orderID)
	if _, body := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{cookie}, "localhost"); strings.Contains(body, cancelPath) {
		t.Fatal("accepted order offers cancel")
	}
	resp, _ := makeRequest(http.MethodPost, cancelPath, nil, []*http.Cookie{cookie}, "localhost")
	assertFlashRedirect(t, resp, "/account", "already being prepared")
	var status string
	if err := testStore.DB.QueryRow("SELECT status FROM orders WHERE id = $1", orderID).Scan(&status); err != nil || status != db.OrderStatusProcessing {
		t.Fatalf("accepted order status=%s err=%v", status, err)
	}

	// The API reports the same refusal.
	resp, body := makeAPIRequestOnHost(t, "localhost", http.MethodPost, fmt.Sprintf("/api/v1/account/orders/%d/cancel", orderID), nil, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("API cancel accepted order status=%d body=%s", resp.StatusCode, body)
	}
}

func TestIntegration_APICancelReturnsUpdatedOrder(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	buyer := fmt.Sprintf("apicancel_%d", suffix)
	buyerID := createTestUser(t, buyer, buyer+"@example.com", "BuyerPass12345", "user")
	productID := createTestProduct(t, "API Mug", fmt.Sprintf("api-mug-%d", suffix), 1200, 5)
	if err := testStore.AddToCart(t.Context(), int(buyerID), int(productID), 1); err != nil {
		t.Fatal(err)
	}
	orderID, err := testStore.PlaceOrderFromCart(t.Context(), int(buyerID), "3 API Street")
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAndGetSessionCookie(t, "localhost", buyer, "BuyerPass12345")
	resp, body := makeAPIRequestOnHost(t, "localhost", http.MethodGet, fmt.Sprintf("/api/v1/account/orders/%d", orderID), nil, []*http.Cookie{cookie}, false, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"cancellable":true`) {
		t.Fatalf("order before cancel: %d %s", resp.StatusCode, body)
	}
	resp, body = makeAPIRequestOnHost(t, "localhost", http.MethodPost, fmt.Sprintf("/api/v1/account/orders/%d/cancel", orderID), nil, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"cancelled"`) || !strings.Contains(string(body), `"cancellable":false`) {
		t.Fatalf("API cancel: %d %s", resp.StatusCode, body)
	}
}
