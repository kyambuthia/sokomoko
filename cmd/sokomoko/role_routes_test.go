package main

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func expectRouteStatus(t *testing.T, method, path, host string, data url.Values, cookies []*http.Cookie, want int) string {
	t.Helper()
	resp, body := makeRequest(method, path, data, cookies, host)
	if resp == nil {
		t.Fatalf("%s %s (host=%s) returned nil response", method, path, host)
	}
	if resp.StatusCode != want {
		if len(body) > 180 {
			body = body[:180]
		}
		t.Fatalf("%s %s (host=%s) status=%d want=%d body=%q", method, path, host, resp.StatusCode, want, body)
	}
	return body
}

func setupPartnerStore(t *testing.T, suffix int64) {
	t.Helper()
	err := testStore.UpsertStoreSettings(db.StoreSettings{
		StoreName:    fmt.Sprintf("Ops Store %d", suffix),
		StoreSlug:    fmt.Sprintf("ops-store-%d", suffix),
		Description:  "integration test store",
		ContactEmail: fmt.Sprintf("ops_%d@example.com", suffix),
	})
	if err != nil {
		t.Fatalf("failed to upsert store settings: %v", err)
	}
}

func createPendingOrderForRoleTests(t *testing.T, suffix int64) int64 {
	t.Helper()
	buyerID := createTestUser(
		t,
		fmt.Sprintf("buyer_%d", suffix),
		fmt.Sprintf("buyer_%d@example.com", suffix),
		"BuyerPass12345",
		"user",
	)
	productID := createTestProduct(
		t,
		"Role Test Product",
		fmt.Sprintf("role-test-product-%d", suffix),
		19.99,
		10,
	)
	if err := testStore.AddToCart(int(buyerID), int(productID), 1); err != nil {
		t.Fatalf("failed to add to cart: %v", err)
	}
	orderID, err := testStore.PlaceOrderFromCart(int(buyerID), "123 Route Matrix Ave")
	if err != nil {
		t.Fatalf("failed to place order: %v", err)
	}
	return orderID
}

func TestIntegration_AdminCanAccessAdminAndStaffOperationalRoutes(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	adminUsername := fmt.Sprintf("admin_%d", suffix)
	adminPassword := "AdminPass12345"
	createTestUser(t, adminUsername, fmt.Sprintf("admin_%d@example.com", suffix), adminPassword, "admin")
	staffToDeactivateID := createTestUser(t, fmt.Sprintf("staff_deactivate_%d", suffix), fmt.Sprintf("staff_deactivate_%d@example.com", suffix), "StaffPass12345", "staff")

	setupPartnerStore(t, suffix)
	orderID := createPendingOrderForRoleTests(t, suffix)

	adminCookie := loginAndGetSessionCookie(t, "admin.localhost", adminUsername, adminPassword)

	adminAllowed := []string{"/", "/products", "/orders", "/reports", "/deliveries", "/team", "/audit", "/staff/signup"}
	for _, path := range adminAllowed {
		expectRouteStatus(t, http.MethodGet, path, "admin.localhost", nil, []*http.Cookie{adminCookie}, http.StatusOK)
	}

	expectRouteStatus(
		t,
		http.MethodPost,
		"/orders",
		"admin.localhost",
		url.Values{
			"order_id":        {fmt.Sprintf("%d", orderID)},
			"status":          {"processing"},
			"partner_status":  {"accepted"},
			"delivery_status": {"processing"},
			"delivery_notice": {"accepted by admin"},
		},
		[]*http.Cookie{adminCookie},
		http.StatusOK,
	)
	expectRouteStatus(
		t,
		http.MethodPost,
		"/orders",
		"admin.localhost",
		url.Values{
			"order_id":        {fmt.Sprintf("%d", orderID)},
			"status":          {"bogus"},
			"partner_status":  {"accepted"},
			"delivery_status": {"processing"},
			"delivery_notice": {"invalid status test"},
		},
		[]*http.Cookie{adminCookie},
		http.StatusBadRequest,
	)

	expectRouteStatus(
		t,
		http.MethodPost,
		"/team",
		"admin.localhost",
		url.Values{"user_id": {fmt.Sprintf("%d", staffToDeactivateID)}},
		[]*http.Cookie{adminCookie},
		http.StatusOK,
	)
	deactivated, err := testStore.GetUserByID(int(staffToDeactivateID))
	if err != nil {
		t.Fatalf("failed to load deactivated user: %v", err)
	}
	if deactivated != nil {
		t.Fatalf("expected staff user %d to be deactivated", staffToDeactivateID)
	}

	rootResp, _ := makeRequest(http.MethodGet, "/", nil, []*http.Cookie{adminCookie}, "partner.localhost")
	if rootResp == nil {
		t.Fatal("GET / on partner.localhost returned nil response")
	}
	if rootResp.StatusCode != http.StatusFound {
		t.Fatalf("GET / on partner.localhost status=%d want=%d", rootResp.StatusCode, http.StatusFound)
	}
	if loc := rootResp.Header.Get("Location"); loc != "/dashboard" {
		t.Fatalf("partner root location=%q want=/dashboard", loc)
	}

	partnerAllowed := []string{"/setup", "/dashboard", "/products", "/products/new", "/orders", "/signup"}
	for _, path := range partnerAllowed {
		expectRouteStatus(t, http.MethodGet, path, "partner.localhost", nil, []*http.Cookie{adminCookie}, http.StatusOK)
	}

	expectRouteStatus(
		t,
		http.MethodPost,
		"/orders",
		"partner.localhost",
		url.Values{
			"order_id":        {fmt.Sprintf("%d", orderID)},
			"partner_status":  {"accepted"},
			"delivery_status": {"processing"},
			"delivery_notice": {"accepted by partner"},
		},
		[]*http.Cookie{adminCookie},
		http.StatusOK,
	)

	newStaffUsername := fmt.Sprintf("staff_new_%d", suffix)
	expectRouteStatus(
		t,
		http.MethodPost,
		"/signup",
		"partner.localhost",
		url.Values{
			"username": {newStaffUsername},
			"email":    {fmt.Sprintf("staff_new_%d@example.com", suffix)},
			"password": {"StaffCreate12345"},
		},
		[]*http.Cookie{adminCookie},
		http.StatusOK,
	)
	createdStaff, err := testStore.GetUserByUsername(newStaffUsername)
	if err != nil {
		t.Fatalf("failed to load newly created staff: %v", err)
	}
	if createdStaff == nil || createdStaff.Role != "staff" {
		t.Fatalf("expected newly created user %q to have staff role", newStaffUsername)
	}
}

func TestIntegration_StaffCanAccessStaffRoutesAndBlockedFromAdminOnly(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	createTestUser(t, fmt.Sprintf("admin_%d", suffix), fmt.Sprintf("admin_%d@example.com", suffix), "AdminPass12345", "admin")
	staffUsername := fmt.Sprintf("staff_%d", suffix)
	staffPassword := "StaffPass12345"
	createTestUser(t, staffUsername, fmt.Sprintf("staff_%d@example.com", suffix), staffPassword, "staff")

	setupPartnerStore(t, suffix)
	orderID := createPendingOrderForRoleTests(t, suffix)

	staffCookie := loginAndGetSessionCookie(t, "admin.localhost", staffUsername, staffPassword)

	adminStaffAllowed := []string{"/", "/products", "/orders", "/reports", "/deliveries"}
	for _, path := range adminStaffAllowed {
		expectRouteStatus(t, http.MethodGet, path, "admin.localhost", nil, []*http.Cookie{staffCookie}, http.StatusOK)
	}

	expectRouteStatus(
		t,
		http.MethodPost,
		"/orders",
		"admin.localhost",
		url.Values{
			"order_id":        {fmt.Sprintf("%d", orderID)},
			"status":          {"processing"},
			"partner_status":  {"accepted"},
			"delivery_status": {"processing"},
			"delivery_notice": {"updated by staff"},
		},
		[]*http.Cookie{staffCookie},
		http.StatusForbidden,
	)

	adminOnly := []string{"/team", "/audit", "/staff/signup"}
	for _, path := range adminOnly {
		expectRouteStatus(t, http.MethodGet, path, "admin.localhost", nil, []*http.Cookie{staffCookie}, http.StatusForbidden)
	}

	partnerStaffAllowed := []string{"/setup", "/dashboard", "/products", "/products/new", "/orders"}
	for _, path := range partnerStaffAllowed {
		expectRouteStatus(t, http.MethodGet, path, "partner.localhost", nil, []*http.Cookie{staffCookie}, http.StatusOK)
	}

	expectRouteStatus(
		t,
		http.MethodPost,
		"/orders",
		"partner.localhost",
		url.Values{
			"order_id":        {fmt.Sprintf("%d", orderID)},
			"partner_status":  {"accepted"},
			"delivery_status": {"processing"},
			"delivery_notice": {"partner queue updated"},
		},
		[]*http.Cookie{staffCookie},
		http.StatusOK,
	)

	expectRouteStatus(t, http.MethodGet, "/signup", "partner.localhost", nil, []*http.Cookie{staffCookie}, http.StatusForbidden)
	expectRouteStatus(
		t,
		http.MethodPost,
		"/signup",
		"partner.localhost",
		url.Values{
			"username": {fmt.Sprintf("forbidden_staff_create_%d", suffix)},
			"email":    {fmt.Sprintf("forbidden_%d@example.com", suffix)},
			"password": {"Forbidden12345"},
		},
		[]*http.Cookie{staffCookie},
		http.StatusForbidden,
	)
}
