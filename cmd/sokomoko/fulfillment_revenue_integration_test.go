package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

func payCustomerID(t *testing.T) int {
	t.Helper()
	var id int
	if err := testStore.DB.QueryRow(`SELECT id FROM users WHERE username = 'pay_customer'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func latestCustomerOrder(t *testing.T) *db.Order {
	t.Helper()
	orders, err := testStore.ListOrdersByUser(t.Context(), payCustomerID(t))
	if err != nil || len(orders) == 0 {
		t.Fatalf("customer orders: %v", err)
	}
	return &orders[0]
}

func stockOnHandAllocated(t *testing.T, productID int64) (onHand, allocated int) {
	t.Helper()
	if err := testStore.DB.QueryRow(
		`SELECT on_hand_quantity, allocated_quantity FROM inventory_stocks WHERE product_id = $1`, productID,
	).Scan(&onHand, &allocated); err != nil {
		t.Fatal(err)
	}
	return onHand, allocated
}

func postAdminOrder(t *testing.T, admin *http.Cookie, orderID int, status, partnerStatus, deliveryStatus string) (*http.Response, string) {
	t.Helper()
	return makeRequest(http.MethodPost, "/orders", url.Values{
		"order_id":        {fmt.Sprint(orderID)},
		"status":          {status},
		"partner_status":  {partnerStatus},
		"delivery_status": {deliveryStatus},
	}, []*http.Cookie{admin}, "admin.localhost")
}

func addLampToCart(t *testing.T, customer *http.Cookie, productID int64) {
	t.Helper()
	resp, _ := makeRequest(http.MethodPost, "/cart/add", url.Values{
		"product_id": {fmt.Sprint(productID)},
		"quantity":   {"1"},
	}, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("add to cart status=%d", resp.StatusCode)
	}
}

// confirmMpesa drives the real callback for the STK prompt just started.
func confirmMpesa(t *testing.T, orderID int, charged money.Cents, receipt string) {
	t.Helper()
	callbackURL, _ := testProviders.lastSTK["CallBackURL"].(string)
	callbackPath := strings.TrimPrefix(callbackURL, "https://shop.example")
	checkoutID := fmt.Sprintf("ws_CO_%d", testProviders.stkCount)
	payload := fmt.Sprintf(`{"Body":{"stkCallback":{"MerchantRequestID":"m-%d","CheckoutRequestID":%q,"ResultCode":0,"ResultDesc":"ok","CallbackMetadata":{"Item":[{"Name":"Amount","Value":%d},{"Name":"MpesaReceiptNumber","Value":%q}]}}}}`,
		testProviders.stkCount, checkoutID, int64(charged)/100, receipt)
	testProviders.mu.Lock()
	testProviders.stkResult = "paid"
	testProviders.mu.Unlock()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+callbackPath, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || paymentStatus(t, orderID) != db.PaymentStatusCaptured {
		t.Fatalf("capture status=%d payment=%s body=%s", rec.Code, paymentStatus(t, orderID), rec.Body.String())
	}
}

func TestIntegration_AdminCannotShipUnpaidOnlineOrder(t *testing.T) {
	customer, admin, productID := paymentsShop(t, "KES", 100000)
	enableProvider(t, admin, "mpesa", mpesaFields)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})
	order := latestCustomerOrder(t)
	if order.PaymentMethod != "mpesa" || order.PaymentStatus != db.PaymentStatusPending {
		t.Fatalf("placed order = %+v", order)
	}
	onHand, allocated := stockOnHandAllocated(t, productID)
	if onHand != 5 || allocated != 1 {
		t.Fatalf("stock before admin update on_hand=%d allocated=%d", onHand, allocated)
	}

	for _, move := range []struct{ status, partner, delivery string }{
		{"shipped", "dispatched", "shipped"},
		{"delivered", "completed", "delivered"},
		{"processing", "accepted", "processing"},
	} {
		resp, body := postAdminOrder(t, admin, order.ID, move.status, move.partner, move.delivery)
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Order state is invalid") {
			t.Fatalf("admin %s status=%d body missing refusal", move.status, resp.StatusCode)
		}
	}
	reloaded, err := testStore.GetOrderForUser(t.Context(), order.UserID, order.ID)
	if err != nil || reloaded == nil {
		t.Fatal(err)
	}
	if reloaded.Status != db.OrderStatusPending || reloaded.InventoryState != "allocated" || reloaded.PartnerStatus != "new" {
		t.Fatalf("unpaid order changed: %+v", reloaded)
	}
	onHand, allocated = stockOnHandAllocated(t, productID)
	if onHand != 5 || allocated != 1 {
		t.Fatalf("stock after refused admin update on_hand=%d allocated=%d", onHand, allocated)
	}

	resp, _ := makeRequest(http.MethodPost, fmt.Sprintf("/account/orders/%d/cancel", order.ID), nil, []*http.Cookie{customer}, "")
	assertFlashRedirect(t, resp, "/account", fmt.Sprintf("Order #%d cancelled", order.ID))
	reloaded, err = testStore.GetOrderForUser(t.Context(), order.UserID, order.ID)
	if err != nil || reloaded == nil || reloaded.Status != db.OrderStatusCancelled || reloaded.InventoryState != "released" {
		t.Fatalf("after customer cancel: %+v %v", reloaded, err)
	}
	if got := availableStock(t, productID); got != 5 {
		t.Fatalf("available after cancel = %d, want 5", got)
	}
	onHand, allocated = stockOnHandAllocated(t, productID)
	if onHand != 5 || allocated != 0 {
		t.Fatalf("stock after cancel on_hand=%d allocated=%d", onHand, allocated)
	}
}

func TestIntegration_AdminFulfillsCashOnDelivery(t *testing.T) {
	customer, admin, productID := paymentsShop(t, "KES", 100000)
	placeOrderCOD(t, customer)
	order := latestCustomerOrder(t)
	if order.PaymentMethod != "cash_on_delivery" || order.Status != db.OrderStatusPending {
		t.Fatalf("cash order = %+v", order)
	}

	resp, body := postAdminOrder(t, admin, order.ID, "shipped", "dispatched", "shipped")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("cash ship status=%d body=%s", resp.StatusCode, body)
	}
	assertFlashRedirect(t, resp, "/orders", "Order updated")
	reloaded, err := testStore.GetOrderForUser(t.Context(), order.UserID, order.ID)
	if err != nil || reloaded == nil || reloaded.Status != db.OrderStatusShipped || reloaded.InventoryState != "shipped" {
		t.Fatalf("shipped cash order: %+v %v", reloaded, err)
	}
	onHand, allocated := stockOnHandAllocated(t, productID)
	if onHand != 4 || allocated != 0 {
		t.Fatalf("stock after cash ship on_hand=%d allocated=%d", onHand, allocated)
	}

	resp, body = postAdminOrder(t, admin, order.ID, "delivered", "completed", "delivered")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("cash deliver status=%d body=%s", resp.StatusCode, body)
	}
	reloaded, err = testStore.GetOrderForUser(t.Context(), order.UserID, order.ID)
	if err != nil || reloaded == nil || reloaded.Status != db.OrderStatusDelivered || reloaded.InventoryState != "shipped" {
		t.Fatalf("delivered cash order: %+v %v", reloaded, err)
	}
}

func TestIntegration_SalesReportCountsCapturedPaymentsAndCashOrders(t *testing.T) {
	customer, admin, productID := paymentsShop(t, "KES", 100000)
	enableProvider(t, admin, "mpesa", mpesaFields)

	placeOrderCOD(t, customer)
	cashKept := latestCustomerOrder(t)

	addLampToCart(t, customer, productID)
	placeOrderCOD(t, customer)
	cashCancelled := latestCustomerOrder(t)
	resp, _ := makeRequest(http.MethodPost, fmt.Sprintf("/account/orders/%d/cancel", cashCancelled.ID), nil, []*http.Cookie{customer}, "")
	assertFlashRedirect(t, resp, "/account", fmt.Sprintf("Order #%d cancelled", cashCancelled.ID))

	addLampToCart(t, customer, productID)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})
	unpaid := latestCustomerOrder(t)

	addLampToCart(t, customer, productID)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})
	paid := latestCustomerOrder(t)
	payments, err := testStore.ListPaymentsByOrderID(t.Context(), paid.ID)
	if err != nil || len(payments) != 1 {
		t.Fatalf("payments: %+v %v", payments, err)
	}
	confirmMpesa(t, paid.ID, payments[0].Amount, fmt.Sprintf("RCPT-%d", paid.ID))

	orders, err := testStore.ListOrdersByUser(t.Context(), payCustomerID(t))
	if err != nil {
		t.Fatal(err)
	}
	var want money.Cents
	for _, order := range orders {
		rows, err := testStore.ListPaymentsByOrderID(t.Context(), order.ID)
		if err != nil {
			t.Fatal(err)
		}
		cash := false
		for _, payment := range rows {
			if payment.Method != "cash_on_delivery" && payment.Status == db.PaymentStatusCaptured {
				want += payment.Amount
			}
			if payment.Method == "cash_on_delivery" {
				cash = true
			}
		}
		if cash && order.Status != db.OrderStatusCancelled {
			want += order.TotalAmount
		}
	}
	if want != cashKept.TotalAmount+payments[0].Amount {
		t.Fatalf("rule total %d, cash %d + captured %d", want, cashKept.TotalAmount, payments[0].Amount)
	}
	if unpaid.ID == paid.ID || cashCancelled.Status == "" {
		t.Fatal("fixture orders were not distinct")
	}

	reportResp, reportBody := makeAPIRequestOnHost(t, "admin.localhost", http.MethodGet, "/api/v1/admin/reports", nil, []*http.Cookie{admin}, false, nil)
	if reportResp.StatusCode != http.StatusOK {
		t.Fatalf("api report status=%d body=%s", reportResp.StatusCode, reportBody)
	}
	var payload struct {
		Data struct {
			RevenueTotal float64 `json:"revenue_total"`
			OrderCount   int     `json:"order_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reportBody, &payload); err != nil {
		t.Fatal(err)
	}
	if got := money.Cents(payload.Data.RevenueTotal*100 + 0.5); got != want {
		t.Fatalf("api revenue = %d, want %d", got, want)
	}
	if payload.Data.OrderCount != len(orders) {
		t.Fatalf("order count = %d, want %d", payload.Data.OrderCount, len(orders))
	}

	pageResp, page := makeRequest(http.MethodGet, "/reports", nil, []*http.Cookie{admin}, "admin.localhost")
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("reports page status=%d", pageResp.StatusCode)
	}
	if !strings.Contains(page, "Revenue counts captured online payments plus cash-on-delivery orders that are not cancelled.") {
		t.Fatal("sales report does not state the revenue rule")
	}
	if strings.Contains(page, "Revenue counts orders that are not cancelled.") {
		t.Fatal("sales report still uses the old revenue sentence")
	}
	if !strings.Contains(page, money.Format(want, "KES")) {
		t.Fatalf("reports page missing %s", money.Format(want, "KES"))
	}
}
