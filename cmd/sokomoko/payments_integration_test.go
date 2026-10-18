package main

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/lifecycle"
)

var testProviders *fakeProviders

// fakeProviders imitates the Paystack and M-Pesa Daraja endpoints the store
// calls, and records what it was sent.
type fakeProviders struct {
	server *httptest.Server

	mu          sync.Mutex
	stkResult   string // "processing", "paid", or "cancelled"
	lastSTK     map[string]any
	stkCount    int
	paystackAmt map[string]int64 // reference -> amount initialized
	paystackCur map[string]string
}

func newFakeProviders() *fakeProviders {
	f := &fakeProviders{stkResult: "processing", paystackAmt: map[string]int64{}, paystackCur: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

func (f *fakeProviders) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stkResult, f.lastSTK, f.stkCount = "processing", nil, 0
}

func (f *fakeProviders) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/oauth/v1/generate":
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":"3599"}`))
	case r.URL.Path == "/mpesa/stkpush/v1/processrequest":
		_ = json.NewDecoder(r.Body).Decode(&f.lastSTK)
		f.stkCount++
		fmt.Fprintf(w, `{"MerchantRequestID":"m-%d","CheckoutRequestID":"ws_CO_%d","ResponseCode":"0","CustomerMessage":"Success"}`, f.stkCount, f.stkCount)
	case r.URL.Path == "/mpesa/stkpushquery/v1/query":
		switch f.stkResult {
		case "paid":
			_, _ = w.Write([]byte(`{"ResponseCode":"0","ResultCode":"0","ResultDesc":"processed"}`))
		case "cancelled":
			_, _ = w.Write([]byte(`{"ResponseCode":"0","ResultCode":"1032","ResultDesc":"Request cancelled by user"}`))
		default:
			_, _ = w.Write([]byte(`{"errorCode":"500.001.1001","errorMessage":"The transaction is being processed"}`))
		}
	case r.URL.Path == "/transaction/initialize":
		var body struct {
			Amount    int64  `json:"amount"`
			Currency  string `json:"currency"`
			Reference string `json:"reference"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.paystackAmt[body.Reference], f.paystackCur[body.Reference] = body.Amount, body.Currency
		fmt.Fprintf(w, `{"status":true,"data":{"authorization_url":"https://checkout.paystack.test/%s","access_code":"ac","reference":"%s"}}`, body.Reference, body.Reference)
	case strings.HasPrefix(r.URL.Path, "/transaction/verify/"):
		ref := strings.TrimPrefix(r.URL.Path, "/transaction/verify/")
		amount, ok := f.paystackAmt[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":false,"message":"Transaction reference not found"}`))
			return
		}
		fmt.Fprintf(w, `{"status":true,"data":{"status":"success","amount":%d,"currency":"%s","reference":"%s","id":7}}`, amount, f.paystackCur[ref], ref)
	default:
		http.NotFound(w, r)
	}
}

// paymentsShop sets up an admin, a customer with one product in the cart,
// and the given currency, then returns the customer and admin cookies.
func paymentsShop(t *testing.T, currency string, price int64) (customer, admin *http.Cookie, productID int64) {
	t.Helper()
	clearAllTables()
	testProviders.reset()
	suffix := time.Now().UnixNano() % 1_000_000_000
	createTestUser(t, "pay_admin", "pay_admin@example.com", "adminpass123", "admin")
	createTestUser(t, "pay_customer", "pay_customer@example.com", "custpass1234", "user")
	if _, err := testStore.DB.Exec(`INSERT INTO store_settings (id, store_name, store_slug, contact_email) VALUES (1, 'Store', 'store', 'store@example.com')`); err != nil {
		t.Fatal(err)
	}
	admin = loginAndGetSessionCookie(t, "admin.localhost", "pay_admin", "adminpass123")
	resp, body := makeRequest(http.MethodPost, "/payments", url.Values{
		"action": {"pricing"}, "currency": {currency}, "shipping_flat": {"0"}, "free_shipping_threshold": {"0"}, "tax_rate": {"0"},
	}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("save pricing status=%d body=%s", resp.StatusCode, body)
	}
	productID = createTestProduct(t, "Payment Lamp", fmt.Sprintf("payment-lamp-%d", suffix), 0, 5)
	if _, err := testStore.DB.Exec("UPDATE products SET price_cents = $1 WHERE id = $2", price, productID); err != nil {
		t.Fatal(err)
	}
	customer = loginAndGetSessionCookie(t, "", "pay_customer", "custpass1234")
	if resp, _ := makeRequest(http.MethodPost, "/cart/add", url.Values{"product_id": {fmt.Sprint(productID)}, "quantity": {"1"}}, []*http.Cookie{customer}, ""); resp.StatusCode != http.StatusFound {
		t.Fatalf("add to cart status=%d", resp.StatusCode)
	}
	return customer, admin, productID
}

func enableProvider(t *testing.T, admin *http.Cookie, provider string, fields map[string]string) {
	t.Helper()
	form := url.Values{"action": {"provider"}, "provider": {provider}, "enabled": {"1"}, "mode": {"test"}}
	for k, v := range fields {
		form.Set("field_"+k, v)
	}
	resp, body := makeRequest(http.MethodPost, "/payments", form, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("enable %s status=%d body=%s", provider, resp.StatusCode, body)
	}
}

var mpesaFields = map[string]string{
	"consumer_key": "ck", "consumer_secret": "cs", "passkey": "pk", "shortcode": "174379", "transaction_type": "CustomerPayBillOnline",
}

func placeOrder(t *testing.T, customer *http.Cookie, method string, extra url.Values) *http.Response {
	t.Helper()
	key := checkoutIdempotencyKey(t, []*http.Cookie{customer})
	form := url.Values{"delivery_address": {"1 Pay Street"}, "payment_method": {method}, "idempotency_key": {key}}
	for k, v := range extra {
		form[k] = v
	}
	resp, body := makeRequest(http.MethodPost, "/checkout", form, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("checkout with %s status=%d body=%s", method, resp.StatusCode, body)
	}
	return resp
}

func paymentStatus(t *testing.T, orderID int) string {
	t.Helper()
	var status string
	if err := testStore.DB.QueryRow("SELECT status FROM payments WHERE order_id = $1", orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func partnerQueueHasOrders(t *testing.T, admin *http.Cookie) bool {
	t.Helper()
	orders, err := testStore.ListOrdersForFulfillment()
	if err != nil {
		t.Fatal(err)
	}
	return len(orders) > 0
}

func TestIntegration_MpesaCheckoutCallbackAndCapture(t *testing.T) {
	customer, admin, _ := paymentsShop(t, "KES", 97092)

	// M-Pesa is refused until the store charges KES; here it does.
	enableProvider(t, admin, "mpesa", mpesaFields)
	_, body := makeRequest(http.MethodGet, "/checkout", nil, []*http.Cookie{customer}, "")
	if !strings.Contains(body, "M-Pesa (Daraja)") || !strings.Contains(body, `name="phone"`) || !strings.Contains(body, "KES 970.92") {
		t.Fatal("checkout does not offer M-Pesa in KES")
	}

	// A bad phone number keeps the customer on checkout without an order.
	key := checkoutIdempotencyKey(t, []*http.Cookie{customer})
	resp, body := makeRequest(http.MethodPost, "/checkout", url.Values{
		"delivery_address": {"1 Pay Street"}, "payment_method": {"mpesa"}, "phone": {"12345"}, "idempotency_key": {key},
	}, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "valid M-Pesa number") {
		t.Fatalf("bad phone status=%d", resp.StatusCode)
	}

	resp = placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712 345 678"}})
	if loc := resp.Header.Get("Location"); loc != "/payments/order/1" {
		t.Fatalf("redirect = %q", loc)
	}
	stk := testProviders.lastSTK
	if stk["Amount"] != float64(971) || stk["PhoneNumber"] != "254712345678" {
		t.Fatalf("STK push = %v", stk)
	}
	callbackURL, _ := stk["CallBackURL"].(string)
	callbackPath := strings.TrimPrefix(callbackURL, "https://shop.example")
	if !strings.HasPrefix(callbackPath, "/payments/callback/mpesa/") {
		t.Fatalf("callback URL = %q", callbackURL)
	}
	if paymentStatus(t, 1) != "pending" || partnerQueueHasOrders(t, admin) {
		t.Fatal("unpaid M-Pesa order must stay out of the fulfillment queue")
	}
	_, body = makeRequest(http.MethodGet, "/payments/order/1", nil, []*http.Cookie{customer}, "")
	if !strings.Contains(body, "enter your M-Pesa PIN") || !strings.Contains(body, "KES 971.00") {
		t.Fatal("payment page lacks the phone prompt or the rounded charge")
	}

	callback := `{"Body":{"stkCallback":{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"ok",
		"CallbackMetadata":{"Item":[{"Name":"Amount","Value":971},{"Name":"MpesaReceiptNumber","Value":"NLJ7RT61SV"}]}}}}`
	postCallback := func(path, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		testHandler.ServeHTTP(rec, req)
		return rec
	}
	if rec := postCallback("/payments/callback/mpesa/not-the-token", callback); rec.Code != http.StatusNotFound {
		t.Fatalf("forged callback status=%d", rec.Code)
	}
	// Daraja still reports the request as processing: no capture yet.
	if rec := postCallback(callbackPath, callback); rec.Code != http.StatusOK || paymentStatus(t, 1) != "pending" {
		t.Fatalf("unconfirmed callback captured: status=%d payment=%s", rec.Code, paymentStatus(t, 1))
	}
	testProviders.mu.Lock()
	testProviders.stkResult = "paid"
	testProviders.mu.Unlock()
	rec := postCallback(callbackPath, callback)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ResultCode":0`) {
		t.Fatalf("callback status=%d body=%s", rec.Code, rec.Body.String())
	}
	if paymentStatus(t, 1) != "captured" || !partnerQueueHasOrders(t, admin) {
		t.Fatal("confirmed M-Pesa payment was not captured or the order is not fulfillable")
	}
	resp, _ = makeRequest(http.MethodGet, "/payments/order/1", nil, []*http.Cookie{customer}, "")
	assertFlashRedirect(t, resp, "/account#order-1", "Payment received")

	// Replaying the callback is harmless.
	if rec := postCallback(callbackPath, callback); rec.Code != http.StatusOK {
		t.Fatalf("replayed callback status=%d", rec.Code)
	}
}

func TestIntegration_MpesaStatusPollConfirmsWithoutCallback(t *testing.T) {
	customer, admin, _ := paymentsShop(t, "KES", 50000)
	enableProvider(t, admin, "mpesa", mpesaFields)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})

	testProviders.mu.Lock()
	testProviders.stkResult = "cancelled"
	testProviders.mu.Unlock()
	resp, body := makeAPIRequest(t, http.MethodGet, "/api/v1/account/orders/1/payment", nil, []*http.Cookie{customer}, false, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"failed"`) || !strings.Contains(string(body), "cancelled on the phone") {
		t.Fatalf("cancelled poll status=%d body=%s", resp.StatusCode, body)
	}

	// Retrying sends a new prompt; a later poll confirms it.
	resp, _ = makeRequest(http.MethodPost, "/payments/order/1", url.Values{"phone": {"0712345678"}}, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusSeeOther || testProviders.stkCount != 2 {
		t.Fatalf("retry status=%d pushes=%d", resp.StatusCode, testProviders.stkCount)
	}
	testProviders.mu.Lock()
	testProviders.stkResult = "paid"
	testProviders.mu.Unlock()
	_, body = makeAPIRequest(t, http.MethodGet, "/api/v1/account/orders/1/payment", nil, []*http.Cookie{customer}, false, nil)
	if !strings.Contains(string(body), `"status":"paid"`) || paymentStatus(t, 1) != "captured" {
		t.Fatalf("poll after approval body=%s payment=%s", body, paymentStatus(t, 1))
	}
}

func TestIntegration_MpesaRetryPreservesOldCallback(t *testing.T) {
	customer, admin, _ := paymentsShop(t, "KES", 50000)
	enableProvider(t, admin, "mpesa", mpesaFields)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})
	firstURL, _ := testProviders.lastSTK["CallBackURL"].(string)
	firstPath := strings.TrimPrefix(firstURL, "https://shop.example")

	// Repeating Start while the first STK prompt is active must not send a
	// second prompt to the customer's phone.
	if resp, _ := makeRequest(http.MethodPost, "/payments/order/1", url.Values{"phone": {"0712345678"}}, []*http.Cookie{customer}, ""); resp.StatusCode != http.StatusSeeOther || testProviders.stkCount != 1 {
		t.Fatalf("duplicate start status=%d pushes=%d", resp.StatusCode, testProviders.stkCount)
	}

	testProviders.mu.Lock()
	testProviders.stkResult = "cancelled"
	testProviders.mu.Unlock()
	makeAPIRequest(t, http.MethodGet, "/api/v1/account/orders/1/payment", nil, []*http.Cookie{customer}, false, nil)
	if resp, _ := makeRequest(http.MethodPost, "/payments/order/1", url.Values{"phone": {"0712345678"}}, []*http.Cookie{customer}, ""); resp.StatusCode != http.StatusSeeOther || testProviders.stkCount != 2 {
		t.Fatalf("retry status=%d pushes=%d", resp.StatusCode, testProviders.stkCount)
	}

	// A delayed success for the original request remains addressable after
	// the retry. The paid attempt wins and the newer prompt is closed.
	testProviders.mu.Lock()
	testProviders.stkResult = "paid"
	testProviders.mu.Unlock()
	callback := `{"Body":{"stkCallback":{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"ok","CallbackMetadata":{"Item":[{"Name":"Amount","Value":500},{"Name":"MpesaReceiptNumber","Value":"OLD-RECEIPT"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+firstPath, strings.NewReader(callback))
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || paymentStatus(t, 1) != db.PaymentStatusCaptured {
		t.Fatalf("old callback status=%d payment=%s", rec.Code, paymentStatus(t, 1))
	}
	rows, err := testStore.DB.Query("SELECT status FROM payment_attempts WHERE payment_id = 1 AND request_reference LIKE 'skm-%' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var statuses []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 || statuses[0] != "completed" || statuses[1] != "failed" {
		t.Fatalf("attempt statuses = %v", statuses)
	}
}

func TestIntegration_PaystackRedirectWebhookAndReturn(t *testing.T) {
	customer, admin, _ := paymentsShop(t, "NGN", 1250000)
	enableProvider(t, admin, "paystack", map[string]string{"secret_key": "sk_test_paystack"})

	resp := placeOrder(t, customer, "paystack", nil)
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://checkout.paystack.test/skm-") {
		t.Fatalf("checkout redirect = %q", loc)
	}
	reference := strings.TrimPrefix(loc, "https://checkout.paystack.test/")
	if testProviders.paystackAmt[reference] != 1250000 || testProviders.paystackCur[reference] != "NGN" {
		t.Fatalf("initialized %d %s", testProviders.paystackAmt[reference], testProviders.paystackCur[reference])
	}

	webhook := []byte(`{"event":"charge.success","data":{"reference":"` + reference + `"}}`)
	send := func(signature string) int {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/payments/webhooks/paystack", strings.NewReader(string(webhook)))
		req.Header.Set("X-Paystack-Signature", signature)
		rec := httptest.NewRecorder()
		testHandler.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send("forged"); code != http.StatusUnauthorized || paymentStatus(t, 1) != "pending" {
		t.Fatalf("forged webhook status=%d payment=%s", code, paymentStatus(t, 1))
	}
	mac := hmac.New(sha512.New, []byte("sk_test_paystack"))
	mac.Write(webhook)
	if code := send(hex.EncodeToString(mac.Sum(nil))); code != http.StatusOK || paymentStatus(t, 1) != "captured" {
		t.Fatalf("signed webhook status=%d payment=%s", code, paymentStatus(t, 1))
	}

	// The browser return lands on the paid order.
	resp, _ = makeRequest(http.MethodGet, "/payments/return?trxref="+reference+"&reference="+reference, nil, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/payments/order/1" {
		t.Fatalf("return status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestIntegration_UnpaidOnlineOrderExpires(t *testing.T) {
	customer, admin, productID := paymentsShop(t, "KES", 10000)
	enableProvider(t, admin, "mpesa", mpesaFields)
	placeOrder(t, customer, "mpesa", url.Values{"phone": {"0712345678"}})
	callbackURL, _ := testProviders.lastSTK["CallBackURL"].(string)
	callbackPath := strings.TrimPrefix(callbackURL, "https://shop.example")

	if _, err := testStore.DB.Exec("UPDATE payments SET created_at = now() - interval '31 minutes'"); err != nil {
		t.Fatal(err)
	}
	// A recent provider request gets its own settlement window even when the
	// order was placed near the normal expiry deadline.
	cutoff := time.Now().Add(-db.UnpaidOrderTTL)
	if expired, err := testStore.ExpireUnpaidOrders(t.Context(), cutoff); err != nil || expired != 0 {
		t.Fatalf("active attempt expired early: %d, %v", expired, err)
	}
	if _, err := testStore.DB.Exec("UPDATE payment_attempts SET created_at = now() - interval '31 minutes' WHERE status = 'pending'"); err != nil {
		t.Fatal(err)
	}
	expired, err := testStore.ExpireUnpaidOrders(t.Context(), cutoff)
	if err != nil || expired != 1 {
		t.Fatalf("ExpireUnpaidOrders = %d, %v", expired, err)
	}
	var status string
	if err := testStore.DB.QueryRow("SELECT status FROM orders WHERE id = 1").Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("order status = %q, %v", status, err)
	}
	if paymentStatus(t, 1) != "failed" {
		t.Fatal("expired payment should be failed")
	}
	product, err := testStore.GetProductByID(int(productID))
	if err != nil || product.StockQuantity != 5 {
		t.Fatalf("stock after expiry = %+v, %v", product, err)
	}
	_, body := makeRequest(http.MethodGet, "/payments/order/1", nil, []*http.Cookie{customer}, "")
	if !strings.Contains(body, "cancelled because payment was not completed") {
		t.Fatal("payment page does not explain the cancellation")
	}

	// A late provider success is recorded for an operator refund, while the
	// cancelled order stays out of the fulfillment queue.
	testProviders.mu.Lock()
	testProviders.stkResult = "paid"
	testProviders.mu.Unlock()
	callback := `{"Body":{"stkCallback":{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"ok","CallbackMetadata":{"Item":[{"Name":"Amount","Value":100},{"Name":"MpesaReceiptNumber","Value":"LATE-RECEIPT"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+callbackPath, strings.NewReader(callback))
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || paymentStatus(t, 1) != db.PaymentStatusCaptured || partnerQueueHasOrders(t, admin) {
		t.Fatalf("late capture status=%d payment=%s fulfillable=%t", rec.Code, paymentStatus(t, 1), partnerQueueHasOrders(t, admin))
	}
	_, body = makeRequest(http.MethodGet, "/payments/order/1", nil, []*http.Cookie{customer}, "")
	if !strings.Contains(body, "Contact the store for a refund") {
		t.Fatal("late payment page does not explain refund")
	}
}

func TestIntegration_AdminPaymentsValidation(t *testing.T) {
	_, admin, _ := paymentsShop(t, "USD", 1000)
	resp, body := makeRequest(http.MethodPost, "/payments", url.Values{
		"action": {"provider"}, "provider": {"mpesa"}, "enabled": {"1"}, "mode": {"test"},
		"field_consumer_key": {"ck"}, "field_consumer_secret": {"cs"}, "field_passkey": {"pk"}, "field_shortcode": {"1"},
	}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "charges KES") {
		t.Fatalf("mpesa on a USD store status=%d", resp.StatusCode)
	}
	resp, body = makeRequest(http.MethodPost, "/payments", url.Values{"action": {"provider"}, "provider": {"cash_on_delivery"}}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Keep at least one payment method") {
		t.Fatalf("disabling the last method status=%d", resp.StatusCode)
	}

	createTestUser(t, "pay_staff", "pay_staff@example.com", "staffpass123", "staff")
	staff := loginAndGetSessionCookie(t, "admin.localhost", "pay_staff", "staffpass123")
	if resp, _ := makeRequest(http.MethodGet, "/payments", nil, []*http.Cookie{staff}, "admin.localhost"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff /payments status=%d", resp.StatusCode)
	}

	// The currency is locked once an order exists.
	customer := loginAndGetSessionCookie(t, "", "pay_customer", "custpass1234")
	placeOrderCOD(t, customer)
	resp, body = makeRequest(http.MethodPost, "/payments", url.Values{
		"action": {"pricing"}, "currency": {"KES"}, "shipping_flat": {"0"}, "tax_rate": {"0"},
	}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "cannot change after orders exist") {
		t.Fatalf("currency change after an order status=%d", resp.StatusCode)
	}
}

func placeOrderCOD(t *testing.T, customer *http.Cookie) {
	t.Helper()
	key := checkoutIdempotencyKey(t, []*http.Cookie{customer})
	resp, body := makeRequest(http.MethodPost, "/checkout", url.Values{
		"delivery_address": {"1 Pay Street"}, "payment_method": {"cash_on_delivery"}, "idempotency_key": {key},
	}, []*http.Cookie{customer}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("cash checkout status=%d body=%s", resp.StatusCode, body)
	}
}

func TestUnpaidOrderTTLMatches(t *testing.T) {
	if lifecycle.UnpaidOrderTTL != db.UnpaidOrderTTL {
		t.Fatalf("lifecycle.UnpaidOrderTTL %s != db.UnpaidOrderTTL %s", lifecycle.UnpaidOrderTTL, db.UnpaidOrderTTL)
	}
}
