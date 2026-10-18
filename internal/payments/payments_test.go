package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPaystackStartVerifyAndWebhook(t *testing.T) {
	var initBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk_test_abc" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":false,"message":"Invalid key"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/transaction/initialize":
			_ = json.NewDecoder(r.Body).Decode(&initBody)
			_, _ = w.Write([]byte(`{"status":true,"data":{"authorization_url":"https://checkout.paystack.com/x","access_code":"ac","reference":"ref-1"}}`))
		case r.URL.Path == "/transaction/verify/ref-1":
			_, _ = w.Write([]byte(`{"status":true,"data":{"status":"success","amount":97092,"currency":"KES","reference":"ref-1","id":42}}`))
		case r.URL.Path == "/transaction/verify/ref-2":
			_, _ = w.Write([]byte(`{"status":true,"data":{"status":"failed","amount":100,"currency":"KES","reference":"ref-2","gateway_response":"Declined"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := &Paystack{Client: server.Client(), BaseURL: server.URL}
	cfg := Config{Mode: "test", Values: map[string]string{"secret_key": "sk_test_abc"}}
	ctx := context.Background()

	result, err := p.Start(ctx, cfg, StartRequest{PaymentID: 9, Reference: "ref-1", Amount: 97092, Currency: "kes", Email: "a@b.co", ReturnURL: "https://shop/return"})
	if err != nil || result.RedirectURL != "https://checkout.paystack.com/x" {
		t.Fatalf("Start = %+v, %v", result, err)
	}
	if initBody["amount"] != float64(97092) || initBody["currency"] != "KES" || initBody["reference"] != "ref-1" || initBody["callback_url"] != "https://shop/return" {
		t.Fatalf("initialize body = %v", initBody)
	}
	if _, err := p.Start(ctx, cfg, StartRequest{Reference: "r", Amount: 1, Currency: "EUR"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("EUR start err = %v", err)
	}

	v, err := p.Verify(ctx, cfg, VerifyRequest{Reference: "ref-1"})
	if err != nil || v.Status != StatusPaid || v.Amount != 97092 || v.Currency != "KES" || v.Receipt != "42" {
		t.Fatalf("Verify = %+v, %v", v, err)
	}
	v, err = p.Verify(ctx, cfg, VerifyRequest{Reference: "ref-2"})
	if err != nil || v.Status != StatusFailed || v.Reason != "Declined" {
		t.Fatalf("Verify failed = %+v, %v", v, err)
	}
	if _, err := p.Verify(ctx, Config{Values: map[string]string{"secret_key": "sk_test_wrong"}}, VerifyRequest{Reference: "ref-1"}); !errors.Is(err, ErrProviderFailure) {
		t.Fatalf("bad key err = %v", err)
	}

	body := []byte(`{"event":"charge.success","data":{"reference":"ref-1"}}`)
	mac := hmac.New(sha512.New, []byte("sk_test_abc"))
	mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-Paystack-Signature", hex.EncodeToString(mac.Sum(nil)))
	event, err := p.ParseWebhook(cfg, req, body)
	if err != nil || event.Reference != "ref-1" {
		t.Fatalf("ParseWebhook = %+v, %v", event, err)
	}
	req.Header.Set("X-Paystack-Signature", "deadbeef")
	if _, err := p.ParseWebhook(cfg, req, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("forged webhook err = %v", err)
	}

	if err := p.CheckKeyMode(Config{Mode: "live", Values: map[string]string{"secret_key": "sk_test_x"}}); err == nil {
		t.Fatal("live mode accepted a test key")
	}
}

func TestMpesaStartVerifyAndCallback(t *testing.T) {
	tokenCalls := 0
	var stk map[string]any
	queryResult := `{"errorCode":"500.001.1001","errorMessage":"The transaction is being processed"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v1/generate":
			tokenCalls++
			want := "Basic " + base64.StdEncoding.EncodeToString([]byte("key:secret"))
			if r.Header.Get("Authorization") != want {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":"3599"}`))
		case "/mpesa/stkpush/v1/processrequest":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&stk)
			_, _ = w.Write([]byte(`{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResponseCode":"0","CustomerMessage":"Success"}`))
		case "/mpesa/stkpushquery/v1/query":
			_, _ = w.Write([]byte(queryResult))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	m := &Mpesa{Client: server.Client(), BaseURL: server.URL}
	cfg := Config{Mode: "test", Values: map[string]string{
		"consumer_key": "key", "consumer_secret": "secret", "passkey": "pass", "shortcode": "174379",
		"transaction_type": mpesaPayBill, "account_reference": "A very long store reference",
	}}
	ctx := context.Background()

	result, err := m.Start(ctx, cfg, StartRequest{Amount: 97092, Currency: "KES", Phone: "0712 345 678", CallbackURL: "https://shop/cb/token", Description: "Order 12"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Charged != 97100 || result.Data["checkout_request_id"] != "ws_CO_1" || result.Data["phone"] != "254712345678" {
		t.Fatalf("Start = %+v", result)
	}
	if stk["Amount"] != float64(971) || stk["PartyA"] != "254712345678" || stk["PartyB"] != "174379" ||
		stk["AccountReference"] != "A very long " || stk["CallBackURL"] != "https://shop/cb/token" {
		t.Fatalf("STK body = %v", stk)
	}
	timestamp, _ := stk["Timestamp"].(string)
	wantPassword := base64.StdEncoding.EncodeToString([]byte("174379pass" + timestamp))
	if stk["Password"] != wantPassword || len(timestamp) != 14 {
		t.Fatalf("password %v for timestamp %q", stk["Password"], timestamp)
	}
	if ts, err := time.ParseInLocation("20060102150405", timestamp, eat); err != nil || time.Since(ts) > time.Minute || time.Since(ts) < -time.Minute {
		t.Fatalf("timestamp %q is not current EAT time: %v", timestamp, err)
	}

	req := VerifyRequest{Data: result.Data, Charged: result.Charged}
	if v, err := m.Verify(ctx, cfg, req); err != nil || v.Status != StatusPending {
		t.Fatalf("processing Verify = %+v, %v", v, err)
	}
	queryResult = `{"ResponseCode":"0","ResultCode":"1032","ResultDesc":"Request cancelled by user"}`
	if v, _ := m.Verify(ctx, cfg, req); v.Status != StatusFailed || !strings.Contains(v.Reason, "cancelled") {
		t.Fatalf("cancelled Verify = %+v", v)
	}
	queryResult = `{"ResponseCode":"0","ResultCode":"0","ResultDesc":"The service request is processed successfully."}`
	if v, _ := m.Verify(ctx, cfg, req); v.Status != StatusPaid || v.Amount != 97100 || v.Currency != "KES" {
		t.Fatalf("paid Verify = %+v", v)
	}
	if tokenCalls != 1 {
		t.Fatalf("token requested %d times, want cached after the first", tokenCalls)
	}

	if _, err := m.Start(ctx, cfg, StartRequest{Amount: 100, Currency: "USD", Phone: "0712345678"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("USD start err = %v", err)
	}

	callback := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"ok",
		"CallbackMetadata":{"Item":[{"Name":"Amount","Value":971.00},{"Name":"MpesaReceiptNumber","Value":"NLJ7RT61SV"},{"Name":"PhoneNumber","Value":254712345678}]}}}}`)
	event, err := m.ParseWebhook(cfg, nil, callback)
	if err != nil || event.Data["checkout_request_id"] != "ws_CO_1" || event.Result.Status != StatusPaid ||
		event.Result.Amount != 97100 || event.Result.Receipt != "NLJ7RT61SV" {
		t.Fatalf("callback = %+v %+v, %v", event, event.Result, err)
	}
	failed := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"m-1","CheckoutRequestID":"ws_CO_1","ResultCode":1032,"ResultDesc":"Request cancelled by user"}}}`)
	if event, _ := m.ParseWebhook(cfg, nil, failed); event.Result.Status != StatusFailed {
		t.Fatalf("failed callback = %+v", event.Result)
	}
}

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{
		"0712345678": "254712345678", "+254 712 345 678": "254712345678", "254112345678": "254112345678",
		"712345678": "254712345678", "0112-345-678": "254112345678",
	} {
		if got, err := NormalizePhone(in); err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0812345678", "25471234567", "07123456789", "07x2345678", "+1 415 555 0100"} {
		if _, err := NormalizePhone(in); !errors.Is(err, ErrInvalidPhone) {
			t.Errorf("NormalizePhone(%q) accepted", in)
		}
	}
}

func TestRegistryDescriptors(t *testing.T) {
	r := NewRegistry(Options{})
	for _, id := range []string{"cash_on_delivery", "mpesa", "paystack"} {
		if _, ok := r.Get(id); !ok {
			t.Fatalf("registry lacks %s", id)
		}
	}
	mpesa, _ := r.Get("mpesa")
	if d := mpesa.Descriptor(); !d.SupportsCurrency("kes") || d.SupportsCurrency("USD") || !d.NeedsPhone {
		t.Fatalf("mpesa descriptor = %+v", d)
	}
	cod, _ := r.Get("cash_on_delivery")
	if !cod.Descriptor().SupportsCurrency("NGN") || cod.Descriptor().Online {
		t.Fatal("cash on delivery should accept any currency offline")
	}
}
