package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/ui"
	"golang.org/x/crypto/bcrypt"
)

const testDBPath = "./test_integration.db"

var testServer *httptest.Server
var testStore *db.Store
var testTemplates *ui.Templates

func TestMain(m *testing.M) {
	setupTestServer()
	code := m.Run()
	teardownTestServer()
	os.Exit(code)
}

func setupTestServer() {
	var err error
	testStore, err = db.OpenStoreNoSeed(testDBPath)
	if err != nil {
		panic(err)
	}

	testTemplates, err = ui.ParseTemplates()
	if err != nil {
		panic(err)
	}

	a := app.New(testStore, testTemplates, ui.StaticFS)

	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()
	partnerMux := http.NewServeMux()

	routes.RegisterPublic(a, mainMux)
	routes.RegisterAdmin(a, adminMux)
	routes.RegisterPartner(a, partnerMux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.HasPrefix(host, "admin.") {
			adminMux.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(host, "partner.") {
			partnerMux.ServeHTTP(w, r)
			return
		}
		mainMux.ServeHTTP(w, r)
	})

	testServer = httptest.NewServer(handler)
}

func teardownTestServer() {
	if testServer != nil {
		testServer.Close()
	}
	if testStore != nil {
		testStore.Close()
	}
	os.Remove(testDBPath)
}

func clearUsersTable() {
	if testStore != nil {
		testStore.DB.Exec("DELETE FROM users")
	}
}

func clearAllTables() {
	if testStore == nil || testStore.DB == nil {
		return
	}
	testStore.DB.Exec("DELETE FROM order_items")
	testStore.DB.Exec("DELETE FROM orders")
	testStore.DB.Exec("DELETE FROM cart_items")
	testStore.DB.Exec("DELETE FROM carts")
	testStore.DB.Exec("DELETE FROM product_images")
	testStore.DB.Exec("DELETE FROM products")
	testStore.DB.Exec("DELETE FROM categories")
	testStore.DB.Exec("DELETE FROM audit_logs")
	testStore.DB.Exec("DELETE FROM password_reset_tokens")
	testStore.DB.Exec("DELETE FROM sessions")
	testStore.DB.Exec("DELETE FROM users")
	testStore.DB.Exec("DELETE FROM store_settings")
}

func makeRequest(method, path string, data url.Values, cookies []*http.Cookie, host string) (*http.Response, string) {
	var body io.Reader
	if data != nil {
		body = strings.NewReader(data.Encode())
	}

	req, err := http.NewRequest(method, testServer.URL+path, body)
	if err != nil {
		return nil, ""
	}

	if data != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	if host != "" {
		req.Host = host
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, ""
	}

	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	return resp, string(bodyBytes)
}

func getSessionCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_token" {
			return cookie
		}
	}
	return nil
}

func createTestUser(t *testing.T, username, email, password, role string) int64 {
	t.Helper()

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         role,
		Slug:         username,
	}

	userID, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return userID
}

func loginAndGetSessionCookie(t *testing.T, host, username, password string) *http.Cookie {
	t.Helper()
	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, host)
	if resp == nil {
		t.Fatal("login request failed")
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d, expected %d", resp.StatusCode, http.StatusFound)
	}
	cookie := getSessionCookie(resp)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	return cookie
}

func createTestProduct(t *testing.T, name, slug string, price float64, stock int) int64 {
	t.Helper()
	categoryID, err := testStore.CreateCategory(db.Category{
		Name:        "Test Category " + slug,
		Slug:        "test-category-" + slug,
		Description: "test",
	})
	if err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	productID, err := testStore.CreateProduct(db.Product{
		Name:          name,
		Slug:          slug,
		Description:   "test product",
		Price:         price,
		StockQuantity: stock,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	return productID
}

func sqlNullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

func TestIntegration_GetLoginPage(t *testing.T) {
	resp, _ := makeRequest(http.MethodGet, "/login", nil, nil, "")
	if resp == nil {
		t.Fatal("Failed to make request")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /login returned %v, expected %v", resp.StatusCode, http.StatusOK)
	}
}

func TestIntegration_GetSignupPage(t *testing.T) {
	resp, _ := makeRequest(http.MethodGet, "/signup", nil, nil, "")
	if resp == nil {
		t.Fatal("Failed to make request")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /signup returned %v, expected %v", resp.StatusCode, http.StatusOK)
	}
}

func TestIntegration_Signup(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("testuser_%d", timestamp)
	email := fmt.Sprintf("test_%d@example.com", timestamp)
	password := "testpass123"

	data := url.Values{}
	data.Set("username", username)
	data.Set("email", email)
	data.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/signup", data, nil, "")
	if resp == nil {
		t.Fatal("Failed to make request")
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("POST /signup returned %v, expected %v", resp.StatusCode, http.StatusFound)
	}

	location := resp.Header.Get("Location")
	if location != "/login" {
		t.Errorf("Expected redirect to /login, got %s", location)
	}

	user, err := testStore.GetUserByUsername(username)
	if err != nil || user == nil {
		t.Fatalf("User not created in database: %v", err)
	}

	if user.Username != username || user.Email != email {
		t.Errorf("User data mismatch: got %s/%s, expected %s/%s", user.Username, user.Email, username, email)
	}

	if user.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", user.Role)
	}
}

func TestIntegration_Signup_DuplicateUsername(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("testuser_%d", timestamp)
	email1 := fmt.Sprintf("test1_%d@example.com", timestamp)
	email2 := fmt.Sprintf("test2_%d@example.com", timestamp)

	data := url.Values{}
	data.Set("username", username)
	data.Set("email", email1)
	data.Set("password", "testpass123")

	makeRequest(http.MethodPost, "/signup", data, nil, "")

	data.Set("email", email2)
	resp, _ := makeRequest(http.MethodPost, "/signup", data, nil, "")

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Duplicate username returned %v, expected %v", resp.StatusCode, http.StatusConflict)
	}
}

func TestIntegration_Login(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("loginuser_%d", timestamp)
	email := fmt.Sprintf("login_%d@example.com", timestamp)
	password := "loginpass123"

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "user",
		Slug:         username,
	}

	testStore.CreateUser(user)

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, "")
	if resp == nil {
		t.Fatal("Failed to make request")
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("POST /login returned %v, expected %v", resp.StatusCode, http.StatusFound)
	}

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to /, got %s", location)
	}

	sessionCookie := getSessionCookie(resp)
	if sessionCookie == nil {
		t.Error("Session cookie not set")
	} else if sessionCookie.Value == "" {
		t.Error("Session cookie value is empty")
	}

	sess, err := testStore.GetSession(sessionCookie.Value)
	if err != nil || sess == nil {
		t.Fatalf("Session not created in database: %v", err)
	}

	if sess.ExpiresAt.Before(time.Now()) {
		t.Error("Session expiration time is in the past")
	}
}

func TestIntegration_Login_WrongPassword(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("wrongpass_%d", timestamp)
	email := fmt.Sprintf("wrongpass_%d@example.com", timestamp)
	password := "correctpass"

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "user",
		Slug:         username,
	}

	testStore.CreateUser(user)

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", "wrongpassword")

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Wrong password returned %v, expected %v", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestIntegration_Login_NonexistentUser(t *testing.T) {
	clearUsersTable()

	data := url.Values{}
	data.Set("username", "nonexistentuser")
	data.Set("password", "anypass")

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Nonexistent user returned %v, expected %v", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestIntegration_Logout(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("logoutuser_%d", timestamp)
	email := fmt.Sprintf("logout_%d@example.com", timestamp)
	password := "logoutpass"

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "user",
		Slug:         username,
	}

	testStore.CreateUser(user)

	loginData := url.Values{}
	loginData.Set("username", username)
	loginData.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/login", loginData, nil, "")
	sessionCookie := getSessionCookie(resp)

	resp, _ = makeRequest(http.MethodPost, "/logout", nil, []*http.Cookie{sessionCookie}, "")

	if resp.StatusCode != http.StatusFound {
		t.Errorf("POST /logout returned %v, expected %v", resp.StatusCode, http.StatusFound)
	}

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to /, got %s", location)
	}

	sess, err := testStore.GetSession(sessionCookie.Value)
	if err == nil && sess != nil {
		t.Error("Session still exists in database after logout")
	}
}

func TestIntegration_AdminLogin_Success(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	username := fmt.Sprintf("admin_%d", timestamp)
	email := fmt.Sprintf("admin_%d@example.com", timestamp)
	password := "adminpass"

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "admin",
		Slug:         username,
	}

	testStore.CreateUser(user)

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, "admin.localhost")

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Admin login returned %v, expected %v", resp.StatusCode, http.StatusFound)
	}

	sessionCookie := getSessionCookie(resp)
	if sessionCookie == nil {
		t.Error("Session cookie not set for admin login")
	}
}

func TestIntegration_AdminLogin_RegularUser(t *testing.T) {
	clearUsersTable()

	timestamp := time.Now().Unix()
	adminUsername := fmt.Sprintf("admin_%d", timestamp)
	adminEmail := fmt.Sprintf("admin_%d@example.com", timestamp)
	username := fmt.Sprintf("regular_%d", timestamp)
	email := fmt.Sprintf("regular_%d@example.com", timestamp)
	password := "userpass"
	adminPassword := "adminpass"

	adminSaltBytes := make([]byte, 16)
	_, _ = rand.Read(adminSaltBytes)
	adminSalt := base64.URLEncoding.EncodeToString(adminSaltBytes)
	adminHashedPassword, _ := bcrypt.GenerateFromPassword([]byte(adminPassword+adminSalt), bcrypt.DefaultCost)

	adminUser := db.User{
		Username:     adminUsername,
		Email:        adminEmail,
		PasswordHash: string(adminHashedPassword),
		Salt:         adminSalt,
		Role:         "admin",
		Slug:         adminUsername,
	}
	testStore.CreateUser(adminUser)

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "user",
		Slug:         username,
	}

	testStore.CreateUser(user)

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	resp, _ := makeRequest(http.MethodPost, "/login", data, nil, "admin.localhost")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Regular user on admin login returned %v, expected %v", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestIntegration_AdminDashboard_WithAdminSession(t *testing.T) {
	clearUsersTable()

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := base64.URLEncoding.EncodeToString(saltBytes)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("adminpass"+salt), bcrypt.DefaultCost)

	adminUser := db.User{
		Username:     "testadmin",
		Email:        "testadmin@example.com",
		PasswordHash: string(hashedPassword),
		Salt:         salt,
		Role:         "admin",
		Slug:         "testadmin",
	}

	userID, _ := testStore.CreateUser(adminUser)

	sessionToken := "test_admin_session"
	_ = testStore.CreateSession(db.Session{
		ID:        sessionToken,
		UserID:    int(userID),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	cookie := &http.Cookie{Name: "session_token", Value: sessionToken}
	resp, _ := makeRequest(http.MethodGet, "/", nil, []*http.Cookie{cookie}, "admin.localhost")

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Admin dashboard with session returned %v, expected %v", resp.StatusCode, http.StatusOK)
	}
}

func TestIntegration_AdminDashboard_WithoutSession(t *testing.T) {
	clearUsersTable()

	resp, _ := makeRequest(http.MethodGet, "/", nil, nil, "admin.localhost")

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Admin dashboard without session returned %v, expected %v (redirect)", resp.StatusCode, http.StatusFound)
	}

	location := resp.Header.Get("Location")
	if location != "/login" {
		t.Errorf("Expected redirect to /login, got %s", location)
	}
}

func TestIntegration_Signup_MissingFields(t *testing.T) {
	clearUsersTable()

	testCases := []struct {
		name string
		data url.Values
	}{
		{"Missing username", url.Values{"email": {"test@example.com"}, "password": {"testpass"}}},
		{"Missing email", url.Values{"username": {"testuser"}, "password": {"testpass"}}},
		{"Missing password", url.Values{"username": {"testuser"}, "email": {"test@example.com"}}},
		{"All missing", url.Values{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := makeRequest(http.MethodPost, "/signup", tc.data, nil, "")
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s returned %v, expected %v", tc.name, resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

func TestIntegration_Login_MissingFields(t *testing.T) {
	clearUsersTable()

	testCases := []struct {
		name string
		data url.Values
	}{
		{"Missing username", url.Values{"password": {"testpass"}}},
		{"Missing password", url.Values{"username": {"testuser"}}},
		{"All missing", url.Values{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := makeRequest(http.MethodPost, "/login", tc.data, nil, "")
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s returned %v, expected %v", tc.name, resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

func TestIntegration_CartRequiresAuth(t *testing.T) {
	clearAllTables()

	resp, _ := makeRequest(http.MethodGet, "/cart", nil, nil, "")
	if resp == nil {
		t.Fatal("request failed")
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET /cart status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "/login" {
		t.Fatalf("expected redirect /login, got %s", got)
	}
}

func TestIntegration_CartCheckoutFlow(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("buyer_%d", suffix)
	email := fmt.Sprintf("buyer_%d@example.com", suffix)
	password := "strongpass123"
	userID := createTestUser(t, username, email, password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)

	productID := createTestProduct(t, "Checkout Product", fmt.Sprintf("checkout-product-%d", suffix), 15.5, 8)

	addData := url.Values{}
	addData.Set("product_id", fmt.Sprintf("%d", productID))
	addData.Set("quantity", "2")
	resp, _ := makeRequest(http.MethodPost, "/cart/add", addData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /cart/add status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	resp, body := makeRequest(http.MethodGet, "/cart", nil, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /cart status=%d expected=%d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(body, "Checkout Product") {
		t.Fatal("cart page does not include expected product")
	}

	checkoutData := url.Values{}
	checkoutData.Set("delivery_address", "123 Integration Street")
	resp, _ = makeRequest(http.MethodPost, "/checkout", checkoutData, []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("POST /checkout status=%d expected=%d", resp.StatusCode, http.StatusFound)
	}

	orders, err := testStore.ListOrdersByUser(int(userID))
	if err != nil {
		t.Fatalf("list orders failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	if orders[0].Status != "pending" {
		t.Fatalf("expected pending order, got %s", orders[0].Status)
	}
}

func TestIntegration_PartnerOrders_AuthzMatrix(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	createTestUser(t, fmt.Sprintf("admin_%d", suffix), fmt.Sprintf("admin_%d@example.com", suffix), "adminpass123", "admin")
	userID := createTestUser(t, fmt.Sprintf("cust_%d", suffix), fmt.Sprintf("cust_%d@example.com", suffix), "userpass123", "user")
	createTestUser(t, fmt.Sprintf("staff_%d", suffix), fmt.Sprintf("staff_%d@example.com", suffix), "staffpass123", "staff")

	productID := createTestProduct(t, "Partner Queue Product", fmt.Sprintf("partner-queue-%d", suffix), 9.0, 5)
	if err := testStore.AddToCart(int(userID), int(productID), 1); err != nil {
		t.Fatalf("add cart failed: %v", err)
	}
	orderID, err := testStore.PlaceOrderFromCart(int(userID), "456 Partner Lane")
	if err != nil {
		t.Fatalf("place order failed: %v", err)
	}
	if orderID == 0 {
		t.Fatal("expected non-zero order id")
	}

	userSessionToken := fmt.Sprintf("user_session_%d", suffix)
	if err := testStore.CreateSession(db.Session{ID: userSessionToken, UserID: int(userID), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create user session failed: %v", err)
	}

	resp, _ := makeRequest(http.MethodGet, "/orders", nil, []*http.Cookie{{Name: "session_token", Value: userSessionToken}}, "partner.localhost")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("user access to partner /orders status=%d expected=%d", resp.StatusCode, http.StatusForbidden)
	}

	staffCookie := loginAndGetSessionCookie(t, "partner.localhost", fmt.Sprintf("staff_%d", suffix), "staffpass123")
	resp, _ = makeRequest(http.MethodGet, "/orders", nil, []*http.Cookie{staffCookie}, "partner.localhost")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("staff access to partner /orders status=%d expected=%d", resp.StatusCode, http.StatusOK)
	}

	badUpdate := url.Values{}
	badUpdate.Set("order_id", fmt.Sprintf("%d", orderID))
	badUpdate.Set("partner_status", "completed")
	badUpdate.Set("delivery_status", "delivered")
	badUpdate.Set("delivery_notice", "done")
	resp, body := makeRequest(http.MethodPost, "/orders", badUpdate, []*http.Cookie{staffCookie}, "partner.localhost")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid transition update status=%d expected=%d", resp.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(body, "Unable to update order") {
		t.Fatal("expected invalid transition error in response body")
	}

	goodUpdate := url.Values{}
	goodUpdate.Set("order_id", fmt.Sprintf("%d", orderID))
	goodUpdate.Set("partner_status", "accepted")
	goodUpdate.Set("delivery_status", "processing")
	goodUpdate.Set("delivery_notice", "Order accepted")
	resp, _ = makeRequest(http.MethodPost, "/orders", goodUpdate, []*http.Cookie{staffCookie}, "partner.localhost")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid transition update status=%d expected=%d", resp.StatusCode, http.StatusOK)
	}

	orders, err := testStore.ListOrdersForFulfillment()
	if err != nil {
		t.Fatalf("list fulfillment orders failed: %v", err)
	}
	if len(orders) == 0 || orders[0].PartnerStatus == "new" {
		t.Fatal("expected updated partner status after valid transition")
	}

}

func TestIntegration_AdminAudit_ForbiddenForStaff(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	createTestUser(t, fmt.Sprintf("admin_%d", suffix), fmt.Sprintf("admin_%d@example.com", suffix), "adminpass123", "admin")
	createTestUser(t, fmt.Sprintf("staff_%d", suffix), fmt.Sprintf("staff_%d@example.com", suffix), "staffpass123", "staff")

	staffCookie := loginAndGetSessionCookie(t, "admin.localhost", fmt.Sprintf("staff_%d", suffix), "staffpass123")
	resp, _ := makeRequest(http.MethodGet, "/audit", nil, []*http.Cookie{staffCookie}, "admin.localhost")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff access to /audit status=%d expected=%d", resp.StatusCode, http.StatusForbidden)
	}

	adminCookie := loginAndGetSessionCookie(t, "admin.localhost", fmt.Sprintf("admin_%d", suffix), "adminpass123")
	resp, _ = makeRequest(http.MethodGet, "/audit", nil, []*http.Cookie{adminCookie}, "admin.localhost")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin access to /audit status=%d expected=%d", resp.StatusCode, http.StatusOK)
	}
}
