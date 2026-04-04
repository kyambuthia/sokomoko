package main

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegration_CartRejectsQuantityAboveStock(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("stockuser_%d", suffix)
	email := fmt.Sprintf("stock_%d@example.com", suffix)
	password := "strongpass123"
	createTestUser(t, username, email, password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)

	productID := createTestProduct(t, "Low Stock Product", fmt.Sprintf("low-stock-%d", suffix), 12.0, 2)

	addData := url.Values{}
	addData.Set("product_id", fmt.Sprintf("%d", productID))
	addData.Set("quantity", "3")
	resp, _ := makeRequest(http.MethodPost, "/cart/add", addData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/add status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "/cart?error=Requested+quantity+exceeds+available+stock" {
		t.Fatalf("add redirect location=%q expected=%q", got, "/cart?error=Requested+quantity+exceeds+available+stock")
	}

	addData.Set("quantity", "1")
	resp, _ = makeRequest(http.MethodPost, "/cart/add", addData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/add status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	updateData := url.Values{}
	updateData.Set("product_id", fmt.Sprintf("%d", productID))
	updateData.Set("quantity", "5")
	resp, _ = makeRequest(http.MethodPost, "/cart/update", updateData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/update status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "/cart?error=Requested+quantity+exceeds+available+stock" {
		t.Fatalf("update redirect location=%q expected=%q", got, "/cart?error=Requested+quantity+exceeds+available+stock")
	}
}

func TestIntegration_CheckoutAppliesPricingBreakdownAndPaymentMethod(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("buyer_price_%d", suffix)
	email := fmt.Sprintf("buyer_price_%d@example.com", suffix)
	password := "strongpass123"
	userID := createTestUser(t, username, email, password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)

	productID := createTestProduct(t, "Pricing Product", fmt.Sprintf("pricing-product-%d", suffix), 10.0, 10)
	addData := url.Values{}
	addData.Set("product_id", fmt.Sprintf("%d", productID))
	addData.Set("quantity", "2")
	resp, _ := makeRequest(http.MethodPost, "/cart/add", addData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/add status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	checkoutData := url.Values{}
	checkoutData.Set("delivery_address", "Pricing Lane")
	checkoutData.Set("payment_method", "card_placeholder")
	checkoutData.Set("idempotency_key", checkoutIdempotencyKey(t, []*http.Cookie{cookie}))
	resp, _ = makeRequest(http.MethodPost, "/checkout", checkoutData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /checkout status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}
	if got := resp.Header.Get("Location"); !strings.HasPrefix(got, "/account?message=Order+") {
		t.Fatalf("checkout redirect location=%q expected account success redirect", got)
	}

	orders, err := testStore.ListOrdersByUser(int(userID))
	if err != nil {
		t.Fatalf("list orders failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	if math.Abs(orders[0].TotalAmount-28.10) > 0.001 {
		t.Fatalf("order total amount=%.2f expected=28.10", orders[0].TotalAmount)
	}
	if !strings.Contains(orders[0].DeliveryNotice, "Payment method selected: Card (placeholder)") {
		t.Fatalf("expected payment method in delivery notice, got %q", orders[0].DeliveryNotice)
	}

	payments, err := testStore.ListPaymentsByOrderID(orders[0].ID)
	if err != nil {
		t.Fatalf("list payments failed: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(payments))
	}
	if payments[0].Status != "captured" {
		t.Fatalf("payment status=%q expected=%q", payments[0].Status, "captured")
	}
}

func TestIntegration_CheckoutIsIdempotentBySubmissionKey(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("buyer_replay_%d", suffix)
	email := fmt.Sprintf("buyer_replay_%d@example.com", suffix)
	password := "strongpass123"
	userID := createTestUser(t, username, email, password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)

	productID := createTestProduct(t, "Replay Product", fmt.Sprintf("replay-product-%d", suffix), 14.0, 10)
	addData := url.Values{}
	addData.Set("product_id", fmt.Sprintf("%d", productID))
	addData.Set("quantity", "2")
	resp, _ := makeRequest(http.MethodPost, "/cart/add", addData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/add status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	idempotencyKey := checkoutIdempotencyKey(t, []*http.Cookie{cookie})
	checkoutData := url.Values{}
	checkoutData.Set("delivery_address", "Replay Lane")
	checkoutData.Set("payment_method", "card_placeholder")
	checkoutData.Set("idempotency_key", idempotencyKey)

	resp, _ = makeRequest(http.MethodPost, "/checkout", checkoutData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("first POST /checkout status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	resp, _ = makeRequest(http.MethodPost, "/checkout", checkoutData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("second POST /checkout status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	orders, err := testStore.ListOrdersByUser(int(userID))
	if err != nil {
		t.Fatalf("list orders failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order after replay, got %d", len(orders))
	}

	payments, err := testStore.ListPaymentsByOrderID(orders[0].ID)
	if err != nil {
		t.Fatalf("list payments failed: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("expected 1 payment after replay, got %d", len(payments))
	}
}
