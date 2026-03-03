package db

import (
	"database/sql"
	"fmt"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"os"
	"sync"
	"testing"
	"time"
)

const testDBPath = "./test_t.db"

var testStore *Store

func TestMain(m *testing.M) {
	// Setup: Initialize a test database
	setupTestDB()

	// Run tests
	code := m.Run()

	// Teardown: Close and remove the test database
	teardownTestDB()

	os.Exit(code)
}

func setupTestDB() {
	var err error
	_ = os.Remove(testDBPath)
	testStore, err = OpenStoreNoSeed(testDBPath)
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

func TestInitDB(t *testing.T) {
	if testStore == nil || testStore.DB == nil {
		t.Error("DB connection is nil after setup")
	}

	// Verify tables exist
	rows, err := testStore.DB.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		t.Fatalf("Failed to query tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan table name: %v", err)
		}
		tables = append(tables, name)
	}

	expectedTables := map[string]bool{
		"users":      true,
		"categories": true,
		"products":   true,
	}

	for _, table := range tables {
		if expectedTables[table] {
			delete(expectedTables, table)
		}
	}

	if len(expectedTables) > 0 {
		t.Errorf("Missing expected tables: %v", expectedTables)
	}
}

func TestCreateUser(t *testing.T) {
	user := User{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hashedpassword",
		Salt:         "somesalt",
		Role:         "user",
	}

	id, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero ID for new user, got 0")
	}

	// Verify user exists in DB
	retrievedUser, err := testStore.GetUserByID(int(id))
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	if retrievedUser == nil {
		t.Fatal("User not found after creation")
	}

	if retrievedUser.Username != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, retrievedUser.Username)
	}
}

func TestGetUserByUsername(t *testing.T) {
	username := "findme"
	user := User{
		Username:     username,
		Email:        "findme@example.com",
		PasswordHash: "hashed",
		Salt:         "salt",
		Role:         "user",
	}
	_, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for test: %v", err)
	}

	foundUser, err := testStore.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}

	if foundUser == nil {
		t.Fatal("User not found by username")
	}

	if foundUser.Username != username {
		t.Errorf("Expected username %s, got %s", username, foundUser.Username)
	}

	// Test non-existent user
	notFoundUser, err := testStore.GetUserByUsername("nonexistent")
	if err != nil {
		t.Fatalf("GetUserByUsername for non-existent user failed: %v", err)
	}
	if notFoundUser != nil {
		t.Error("Expected nil for non-existent user, got a user")
	}
}

func TestGetUserByID(t *testing.T) {
	user := User{
		Username:     "byiduser",
		Email:        "byid@example.com",
		PasswordHash: "hashed",
		Salt:         "salt",
		Role:         "user",
	}
	id, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for test: %v", err)
	}

	foundUser, err := testStore.GetUserByID(int(id))
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	if foundUser == nil {
		t.Fatal("User not found by ID")
	}

	if foundUser.ID != int(id) {
		t.Errorf("Expected user ID %d, got %d", id, foundUser.ID)
	}

	// Test non-existent user
	notFoundUser, err := testStore.GetUserByID(99999)
	if err != nil {
		t.Fatalf("GetUserByID for non-existent user failed: %v", err)
	}
	if notFoundUser != nil {
		t.Error("Expected nil for non-existent user, got a user")
	}
}

func TestGetProductBySlug(t *testing.T) {
	suffix := time.Now().UnixNano()
	categoryID, err := testStore.CreateCategory(Category{
		Name: "Slug Category",
		Slug: fmt.Sprintf("slug-category-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	productSlug := fmt.Sprintf("slug-product-%d", suffix)
	productID, err := testStore.CreateProduct(Product{
		Name:          "Slug Product",
		Slug:          productSlug,
		Description:   "slug product",
		Price:         12.5,
		StockQuantity: 3,
		CategoryID:    sql.NullInt64{Int64: categoryID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	if productID == 0 {
		t.Fatal("expected non-zero product id")
	}

	product, err := testStore.GetProductBySlug(productSlug)
	if err != nil {
		t.Fatalf("GetProductBySlug failed: %v", err)
	}
	if product == nil {
		t.Fatal("expected product by slug")
	}
	if product.Slug != productSlug {
		t.Fatalf("product slug = %q, want %q", product.Slug, productSlug)
	}

	missing, err := testStore.GetProductBySlug("missing-slug")
	if err != nil {
		t.Fatalf("GetProductBySlug missing failed: %v", err)
	}
	if missing != nil {
		t.Fatal("expected nil for missing slug")
	}
}

func TestUpdateUser(t *testing.T) {
	user := User{
		Username:     "updateuser",
		Email:        "update@example.com",
		PasswordHash: "oldhash",
		Salt:         "oldsalt",
		Role:         "user",
	}
	id, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for update test: %v", err)
	}

	user.ID = int(id)
	user.Email = "updated@example.com"
	user.Role = "admin"

	err = testStore.UpdateUser(user)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	updatedUser, err := testStore.GetUserByID(int(id))
	if err != nil {
		t.Fatalf("GetUserByID after update failed: %v", err)
	}

	if updatedUser.Email != user.Email || updatedUser.Role != user.Role {
		t.Errorf("User update failed. Expected email %s got %s, Expected role %s got %s",
			user.Email, updatedUser.Email, user.Role, updatedUser.Role)
	}
}

func TestDeleteUser(t *testing.T) {
	user := User{
		Username:     "deleteuser",
		Email:        "delete@example.com",
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
	}
	id, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for delete test: %v", err)
	}

	err = testStore.DeleteUser(int(id))
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	deletedUser, err := testStore.GetUserByID(int(id))
	if err != nil {
		t.Fatalf("GetUserByID after delete failed: %v", err)
	}

	if deletedUser != nil {
		t.Error("User found after deletion, expected nil")
	}
}

func TestCartCheckoutAndFulfillmentFlow(t *testing.T) {
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("buyer_%d", suffix)
	email := fmt.Sprintf("buyer_flow_%d@example.com", suffix)

	userID, err := testStore.CreateUser(User{
		Username:     username,
		Email:        email,
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         username,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Flow Category %d", suffix),
		Slug:        fmt.Sprintf("flow-category-%d", suffix),
		Description: "for flow test",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Flow Product",
		Slug:          fmt.Sprintf("flow-product-%d", suffix),
		Description:   "flow product",
		Price:         12.5,
		StockQuantity: 10,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userID), int(productID), 2); err != nil {
		t.Fatalf("add to cart: %v", err)
	}

	items, subtotal, err := testStore.GetCartItems(int(userID))
	if err != nil {
		t.Fatalf("get cart items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 cart item, got %d", len(items))
	}
	if subtotal <= 0 {
		t.Fatalf("expected positive subtotal, got %.2f", subtotal)
	}

	orderID, err := testStore.PlaceOrderFromCart(int(userID), "123 Test Street")
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	if orderID == 0 {
		t.Fatal("expected non-zero order id")
	}

	product, err := testStore.GetProductByID(int(productID))
	if err != nil {
		t.Fatalf("load product: %v", err)
	}
	if product.StockQuantity != 8 {
		t.Fatalf("expected stock 8, got %d", product.StockQuantity)
	}

	orders, err := testStore.ListOrdersByUser(int(userID))
	if err != nil {
		t.Fatalf("list customer orders: %v", err)
	}
	if len(orders) == 0 {
		t.Fatal("expected at least one order")
	}

	if err := testStore.UpdateOrderFulfillment(int(orderID), "accepted", "processing", "Order accepted by partner."); err != nil {
		t.Fatalf("update fulfillment (accepted): %v", err)
	}
	if err := testStore.UpdateOrderFulfillment(int(orderID), "packing", "processing", "Order is being packed."); err != nil {
		t.Fatalf("update fulfillment (packing): %v", err)
	}
	if err := testStore.UpdateOrderFulfillment(int(orderID), "dispatched", "shipped", "Your order is on the way."); err != nil {
		t.Fatalf("update fulfillment: %v", err)
	}

	partnerOrders, err := testStore.ListOrdersForFulfillment()
	if err != nil {
		t.Fatalf("list partner orders: %v", err)
	}
	if len(partnerOrders) == 0 {
		t.Fatal("expected partner orders")
	}

	summary, err := testStore.GetPartnerOrderSummary()
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.DispatchedCount < 1 {
		t.Fatalf("expected at least 1 dispatched order, got %d", summary.DispatchedCount)
	}
}

func TestOrderFulfillmentTransitionValidation(t *testing.T) {
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("buyer_transition_%d", suffix)
	email := fmt.Sprintf("buyer_transition_%d@example.com", suffix)

	userID, err := testStore.CreateUser(User{
		Username:     username,
		Email:        email,
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         username,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Transition Category %d", suffix),
		Slug:        fmt.Sprintf("transition-category-%d", suffix),
		Description: "for transition test",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Transition Product",
		Slug:          fmt.Sprintf("transition-product-%d", suffix),
		Description:   "transition product",
		Price:         11.0,
		StockQuantity: 5,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userID), int(productID), 1); err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	orderID, err := testStore.PlaceOrderFromCart(int(userID), "456 Transition St")
	if err != nil {
		t.Fatalf("place order: %v", err)
	}

	// Invalid jump: new -> completed should fail.
	err = testStore.UpdateOrderFulfillment(int(orderID), "completed", "delivered", "done")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestUpdateOrderFulfillment_OrderNotFound(t *testing.T) {
	err := testStore.UpdateOrderFulfillment(999999, "new", "queued", "no-op")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestUpdateOrderByAdmin_OrderNotFound(t *testing.T) {
	err := testStore.UpdateOrderByAdmin(999999, "pending", "new", "queued", "no-op")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestConcurrentCheckoutStockContention(t *testing.T) {
	suffix := time.Now().UnixNano()

	userA, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("concurrent_a_%d", suffix),
		Email:        fmt.Sprintf("concurrent_a_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("concurrent-a-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}
	userB, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("concurrent_b_%d", suffix),
		Email:        fmt.Sprintf("concurrent_b_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("concurrent-b-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Concurrent Category %d", suffix),
		Slug:        fmt.Sprintf("concurrent-category-%d", suffix),
		Description: "concurrency",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Concurrent Product",
		Slug:          fmt.Sprintf("concurrent-product-%d", suffix),
		Description:   "limited stock",
		Price:         20,
		StockQuantity: 1,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userA), int(productID), 1); err != nil {
		t.Fatalf("add cart A: %v", err)
	}
	if err := testStore.AddToCart(int(userB), int(productID), 1); err != nil {
		t.Fatalf("add cart B: %v", err)
	}

	var wg sync.WaitGroup
	var successCount int
	var failCount int
	var mu sync.Mutex

	tryCheckout := func(userID int) {
		defer wg.Done()
		_, placeErr := testStore.PlaceOrderFromCart(userID, "Concurrent Street")
		mu.Lock()
		defer mu.Unlock()
		if placeErr == nil {
			successCount++
		} else {
			failCount++
		}
	}

	wg.Add(2)
	go tryCheckout(int(userA))
	go tryCheckout(int(userB))
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful checkout, got %d", successCount)
	}
	if failCount != 1 {
		t.Fatalf("expected exactly 1 failed checkout, got %d", failCount)
	}
}

func sqlNullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}
