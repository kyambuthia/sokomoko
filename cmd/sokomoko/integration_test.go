package main

import (
	"crypto/rand"
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

	routes.RegisterPublic(a, mainMux)
	routes.RegisterAdmin(a, adminMux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.HasPrefix(host, "admin.") {
			adminMux.ServeHTTP(w, r)
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
