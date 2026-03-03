package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"golang.org/x/crypto/bcrypt"
)

const testDBPath = "./test_auth.db"

var testStore *db.Store

func TestMain(m *testing.M) {
	setupTestDB()
	code := m.Run()
	teardownTestDB()
	os.Exit(code)
}

func setupTestDB() {
	var err error
	testStore, err = db.OpenStoreNoSeed(testDBPath)
	if err != nil {
		panic(err)
	}
}

func teardownTestDB() {
	if testStore != nil {
		_ = testStore.Close()
	}
	os.Remove(testDBPath)
}

func clearUsersTable() {
	_, err := testStore.DB.Exec("DELETE FROM users")
	if err != nil {
		panic(err)
	}
}

func createTestUser(username, email, password, role string) *db.User {
	user := db.User{
		Username: username,
		Email:    email,
		Role:     role,
		Slug:     username,
	}

	saltBytes := make([]byte, 16)
	rand.Read(saltBytes)
	user.Salt = base64.URLEncoding.EncodeToString(saltBytes)

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+user.Salt), bcrypt.DefaultCost)
	user.PasswordHash = string(hashedPassword)

	id, err := testStore.CreateUser(user)
	if err != nil {
		panic(err)
	}
	user.ID = int(id)
	return &user
}

func TestSignUp(t *testing.T) {
	clearUsersTable()

	tmpl := template.New("signup.html")
	template.Must(tmpl.Parse("{{define \"root_template\"}}Sign Up Page{{end}}"))

	// Test successful signup
	data := url.Values{}
	data.Set("username", "testuser1")
	data.Set("email", "test1@example.com")
	data.Set("password", "password123")
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	SignUp(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusFound)
	}
	if location := rr.Header().Get("Location"); location != "/login" {
		t.Errorf("handler returned wrong redirect location: got %v want %v", location, "/login")
	}

	user, err := testStore.GetUserByUsername("testuser1")
	if err != nil || user == nil {
		t.Fatalf("User not found after signup: %v", err)
	}

	// Test signup with existing username
	data.Set("email", "test_new@example.com") // Change email to avoid email unique constraint
	req = httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()

	SignUp(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusConflict {
		t.Errorf("handler returned wrong status code for existing user: got %v want %v", status, http.StatusConflict)
	}

	// Test signup with missing fields
	data = url.Values{}
	data.Set("username", "testuser3")
	data.Set("password", "password123") // Missing email
	req = httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()

	SignUp(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code for missing fields: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestLogin(t *testing.T) {
	clearUsersTable()
	createTestUser("loginuser", "login@example.com", "loginpass", "user")

	tmpl := template.New("login.html")
	template.Must(tmpl.Parse("{{define \"root_template\"}}Login Page{{end}}"))

	// Test successful login
	data := url.Values{}
	data.Set("username", "loginuser")
	data.Set("password", "loginpass")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	Login(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusFound)
	}
	if location := rr.Header().Get("Location"); location != "/" {
		t.Errorf("handler returned wrong redirect location: got %v want %v", location, "/")
	}
	cookie := rr.Result().Cookies()[0]
	if cookie.Name != "session_token" || cookie.Value == "" {
		t.Error("Session cookie not set or empty")
	}

	// Test login with incorrect password
	data.Set("password", "wrongpass")
	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()

	Login(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code for incorrect password: got %v want %v", status, http.StatusUnauthorized)
	}

	// Test login with non-existent user
	data.Set("username", "nonexistentuser")
	data.Set("password", "anypass")
	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()

	Login(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code for non-existent user: got %v want %v", status, http.StatusUnauthorized)
	}
}

func TestLogin_SetsConfiguredSessionCookieDomain(t *testing.T) {
	clearUsersTable()
	createTestUser("domainuser", "domain@example.com", "domainpass", "user")

	SetSessionCookieDomain(".example.com")
	t.Cleanup(func() {
		SetSessionCookieDomain("")
	})

	tmpl := template.New("login.html")
	template.Must(tmpl.Parse("{{define \"root_template\"}}Login Page{{end}}"))

	data := url.Values{}
	data.Set("username", "domainuser")
	data.Set("password", "domainpass")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	Login(testStore, tmpl).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusFound {
		t.Fatalf("handler returned wrong status code: got %v want %v", status, http.StatusFound)
	}
	result := rr.Result()
	defer result.Body.Close()

	var sessionCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == "session_token" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}
	if sessionCookie.Domain != "example.com" {
		t.Fatalf("session cookie domain = %q, want %q", sessionCookie.Domain, "example.com")
	}
}

func TestMapAccountCreationError(t *testing.T) {
	status, _ := mapAccountCreationError(errors.New("sqlite3: constraint failed: UNIQUE constraint failed: users.username"), "conflict")
	if status != http.StatusConflict {
		t.Fatalf("unique constraint status=%d want=%d", status, http.StatusConflict)
	}

	status, _ = mapAccountCreationError(errors.New("sqlite3: database is locked"), "conflict")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("locked db status=%d want=%d", status, http.StatusServiceUnavailable)
	}

	status, _ = mapAccountCreationError(errors.New("unexpected failure"), "conflict")
	if status != http.StatusInternalServerError {
		t.Fatalf("generic error status=%d want=%d", status, http.StatusInternalServerError)
	}
}

func TestAuthMiddleware(t *testing.T) {
	clearUsersTable()
	user := createTestUser("authuser", "auth@example.com", "authpass", "user")

	var req *http.Request
	var rr *httptest.ResponseRecorder

	// Create a dummy handler to be protected
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userFromCtx := GetUserFromContext(r.Context())
		if userFromCtx == nil || userFromCtx.ID != user.ID {
			t.Errorf("User not correctly added to context")
		}
		fmt.Fprint(w, "Protected content")
	})

	// Test access with valid session
	sessionToken := "valid_session_token"
	_ = testStore.CreateSession(db.Session{ID: sessionToken, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sessionToken})
	rr = httptest.NewRecorder()

	AuthMiddleware(testStore, protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for valid session: got %v want %v", status, http.StatusOK)
	}
	if body := rr.Body.String(); body != "Protected content" {
		t.Errorf("handler returned unexpected body: got %v want %v", body, "Protected content")
	}

	// Test access without session
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	rr = httptest.NewRecorder()

	AuthMiddleware(testStore, protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusFound {
		t.Errorf("handler returned wrong status code for no session: got %v want %v", status, http.StatusFound)
	}
	if location := rr.Header().Get("Location"); location != "/login" {
		t.Errorf("handler returned wrong redirect location: got %v want %v", location, "/login")
	}

	// Test access with expired session
	sessionToken = "expired_session_token"
	_ = testStore.CreateSession(db.Session{ID: sessionToken, UserID: user.ID, ExpiresAt: time.Now().Add(-time.Hour)})
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sessionToken})
	rr = httptest.NewRecorder()

	AuthMiddleware(testStore, protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusFound {
		t.Errorf("handler returned wrong status code for expired session: got %v want %v", status, http.StatusFound)
	}
	if location := rr.Header().Get("Location"); location != "/login" {
		t.Errorf("handler returned wrong redirect location: got %v want %v", location, "/login")
	}
}

func TestRequireRole(t *testing.T) {
	clearUsersTable()
	adminUser := createTestUser("adminuser", "admin@example.com", "adminpass", "admin")
	regularUser := createTestUser("regularuser", "regular@example.com", "regularpass", "user")

	var req *http.Request
	var rr *httptest.ResponseRecorder

	// Dummy handler for protected content
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Role-protected content")
	})

	// Test admin access to admin-only route
	ctx := context.WithValue(context.Background(), userContextKey, adminUser)
	req = httptest.NewRequest(http.MethodGet, "/admin", nil).WithContext(ctx)
	rr = httptest.NewRecorder()

	RequireRole("admin", protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for admin user: got %v want %v", status, http.StatusOK)
	}
	if body := rr.Body.String(); body != "Role-protected content" {
		t.Errorf("handler returned unexpected body: got %v want %v", body, "Role-protected content")
	}

	// Test regular user access to admin-only route
	ctx = context.WithValue(context.Background(), userContextKey, regularUser)
	req = httptest.NewRequest(http.MethodGet, "/admin", nil).WithContext(ctx)
	rr = httptest.NewRecorder()

	RequireRole("admin", protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusForbidden {
		t.Errorf("handler returned wrong status code for regular user on admin route: got %v want %v", status, http.StatusForbidden)
	}

	// Test unauthenticated access to admin-only route
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr = httptest.NewRecorder()

	RequireRole("admin", protectedHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusForbidden {
		t.Errorf("handler returned wrong status code for unauthenticated user on admin route: got %v want %v", status, http.StatusForbidden)
	}
}
