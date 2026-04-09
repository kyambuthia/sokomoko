package db

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"math"
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
	testStore, err = OpenStore(testDBPath)
	if err != nil {
		panic(err)
	}
	if err := testStore.ApplySchema(); err != nil {
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
		"users":              true,
		"categories":         true,
		"products":           true,
		"warehouses":         true,
		"inventory_stocks":   true,
		"stock_reservations": true,
		"stock_movements":    true,
		"checkouts":          true,
		"checkout_lines":     true,
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

func TestGetSession_DoesNotFallbackToRawSessionID(t *testing.T) {
	suffix := time.Now().UnixNano()
	user := User{
		Username:     fmt.Sprintf("session_user_%d", suffix),
		Email:        fmt.Sprintf("session_user_%d@example.com", suffix),
		PasswordHash: "hashed",
		Salt:         "salt",
		Role:         "user",
	}

	userID, err := testStore.CreateUser(user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	rawSessionID := fmt.Sprintf("raw_session_%d", suffix)
	_, err = testStore.DB.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		rawSessionID,
		userID,
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("insert raw session failed: %v", err)
	}

	sess, err := testStore.GetSession(rawSessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if sess != nil {
		t.Fatal("expected raw session ID lookup to fail when only unhashed storage exists")
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

	stocks, err := testStore.ListInventoryStocksByProductID(int(productID))
	if err != nil {
		t.Fatalf("list inventory stocks: %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("expected 1 inventory stock row, got %d", len(stocks))
	}
	if stocks[0].OnHandQuantity != 10 {
		t.Fatalf("expected on-hand 10, got %d", stocks[0].OnHandQuantity)
	}
	if stocks[0].AllocatedQuantity != 2 {
		t.Fatalf("expected allocated 2, got %d", stocks[0].AllocatedQuantity)
	}
	if stocks[0].AvailableQuantity != 8 {
		t.Fatalf("expected available 8, got %d", stocks[0].AvailableQuantity)
	}

	movements, err := testStore.ListStockMovementsByProductID(int(productID))
	if err != nil {
		t.Fatalf("list stock movements: %v", err)
	}
	if len(movements) < 2 {
		t.Fatalf("expected at least 2 stock movements, got %d", len(movements))
	}
	if movements[0].MovementType != stockMovementInitial {
		t.Fatalf("expected first movement %q, got %q", stockMovementInitial, movements[0].MovementType)
	}
	lastMovement := movements[len(movements)-1]
	if lastMovement.MovementType != stockMovementAllocation {
		t.Fatalf("expected last movement %q, got %q", stockMovementAllocation, lastMovement.MovementType)
	}
	if lastMovement.QuantityDelta != -2 {
		t.Fatalf("expected last movement delta -2, got %d", lastMovement.QuantityDelta)
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

func TestCreateProduct_InitializesInventoryStock(t *testing.T) {
	suffix := time.Now().UnixNano()
	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Inventory Category %d", suffix),
		Slug:        fmt.Sprintf("inventory-category-%d", suffix),
		Description: "inventory",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Inventory Product",
		Slug:          fmt.Sprintf("inventory-product-%d", suffix),
		Description:   "inventory product",
		Price:         9.5,
		StockQuantity: 7,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	stocks, err := testStore.ListInventoryStocksByProductID(int(productID))
	if err != nil {
		t.Fatalf("list inventory stocks: %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("expected 1 inventory stock row, got %d", len(stocks))
	}
	if stocks[0].WarehouseID != defaultWarehouseID {
		t.Fatalf("expected default warehouse %d, got %d", defaultWarehouseID, stocks[0].WarehouseID)
	}
	if stocks[0].OnHandQuantity != 7 {
		t.Fatalf("expected on-hand 7, got %d", stocks[0].OnHandQuantity)
	}
	if stocks[0].AvailableQuantity != 7 {
		t.Fatalf("expected available 7, got %d", stocks[0].AvailableQuantity)
	}

	movements, err := testStore.ListStockMovementsByProductID(int(productID))
	if err != nil {
		t.Fatalf("list stock movements: %v", err)
	}
	if len(movements) != 1 {
		t.Fatalf("expected 1 stock movement, got %d", len(movements))
	}
	if movements[0].MovementType != stockMovementInitial {
		t.Fatalf("expected movement type %q, got %q", stockMovementInitial, movements[0].MovementType)
	}
	if movements[0].QuantityDelta != 7 {
		t.Fatalf("expected movement delta 7, got %d", movements[0].QuantityDelta)
	}
}

func TestUpdateProduct_SynchronizesInventoryProjection(t *testing.T) {
	suffix := time.Now().UnixNano()
	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Update Inventory Category %d", suffix),
		Slug:        fmt.Sprintf("update-inventory-category-%d", suffix),
		Description: "inventory update",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Adjustable Product",
		Slug:          fmt.Sprintf("adjustable-product-%d", suffix),
		Description:   "adjustable",
		Price:         15,
		StockQuantity: 4,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	err = testStore.UpdateProduct(Product{
		ID:            int(productID),
		Name:          "Adjustable Product",
		Slug:          fmt.Sprintf("adjustable-product-%d", suffix),
		Description:   "adjusted",
		Price:         16,
		StockQuantity: 9,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("update product: %v", err)
	}

	product, err := testStore.GetProductByID(int(productID))
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.StockQuantity != 9 {
		t.Fatalf("expected projected stock 9, got %d", product.StockQuantity)
	}

	stocks, err := testStore.ListInventoryStocksByProductID(int(productID))
	if err != nil {
		t.Fatalf("list inventory stocks: %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("expected 1 inventory stock row, got %d", len(stocks))
	}
	if stocks[0].OnHandQuantity != 9 {
		t.Fatalf("expected on-hand 9, got %d", stocks[0].OnHandQuantity)
	}

	movements, err := testStore.ListStockMovementsByProductID(int(productID))
	if err != nil {
		t.Fatalf("list stock movements: %v", err)
	}
	if len(movements) != 2 {
		t.Fatalf("expected 2 stock movements, got %d", len(movements))
	}
	if movements[1].MovementType != stockMovementAdjustment {
		t.Fatalf("expected second movement %q, got %q", stockMovementAdjustment, movements[1].MovementType)
	}
	if movements[1].QuantityDelta != 5 {
		t.Fatalf("expected second movement delta 5, got %d", movements[1].QuantityDelta)
	}
}

func TestReserveCartForCheckout_RefreshesReservationsAndProjection(t *testing.T) {
	suffix := time.Now().UnixNano()
	userID, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("reserve_user_%d", suffix),
		Email:        fmt.Sprintf("reserve_user_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("reserve-user-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Reserve Category %d", suffix),
		Slug:        fmt.Sprintf("reserve-category-%d", suffix),
		Description: "reservation",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Reserve Product",
		Slug:          fmt.Sprintf("reserve-product-%d", suffix),
		Description:   "reserve product",
		Price:         18,
		StockQuantity: 4,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userID), int(productID), 2); err != nil {
		t.Fatalf("add to cart: %v", err)
	}

	if err := testStore.ReserveCartForCheckout(int(userID), "reserve-a", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("reserve cart A: %v", err)
	}

	reservations, err := testStore.ListActiveStockReservationsByKey(int(userID), "reserve-a")
	if err != nil {
		t.Fatalf("list reservations A: %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("expected 1 active reservation for key A, got %d", len(reservations))
	}

	product, err := testStore.GetProductByID(int(productID))
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.StockQuantity != 2 {
		t.Fatalf("expected projected available stock 2, got %d", product.StockQuantity)
	}

	if err := testStore.ReserveCartForCheckout(int(userID), "reserve-b", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("reserve cart B: %v", err)
	}

	reservations, err = testStore.ListActiveStockReservationsByKey(int(userID), "reserve-a")
	if err != nil {
		t.Fatalf("list reservations A after refresh: %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("expected 0 active reservations for key A after refresh, got %d", len(reservations))
	}

	reservations, err = testStore.ListActiveStockReservationsByKey(int(userID), "reserve-b")
	if err != nil {
		t.Fatalf("list reservations B: %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("expected 1 active reservation for key B, got %d", len(reservations))
	}

	stocks, err := testStore.ListInventoryStocksByProductID(int(productID))
	if err != nil {
		t.Fatalf("list inventory stocks: %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("expected 1 inventory stock row, got %d", len(stocks))
	}
	if stocks[0].ReservedQuantity != 2 {
		t.Fatalf("expected reserved quantity 2, got %d", stocks[0].ReservedQuantity)
	}
}

func TestReserveCartForCheckout_ExpiresStaleReservations(t *testing.T) {
	suffix := time.Now().UnixNano()
	userA, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("expire_a_%d", suffix),
		Email:        fmt.Sprintf("expire_a_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("expire-a-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}
	userB, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("expire_b_%d", suffix),
		Email:        fmt.Sprintf("expire_b_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("expire-b-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Expire Category %d", suffix),
		Slug:        fmt.Sprintf("expire-category-%d", suffix),
		Description: "expire reservation",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Expire Product",
		Slug:          fmt.Sprintf("expire-product-%d", suffix),
		Description:   "expire product",
		Price:         20,
		StockQuantity: 1,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userA), int(productID), 1); err != nil {
		t.Fatalf("add to cart A: %v", err)
	}
	if err := testStore.AddToCart(int(userB), int(productID), 1); err != nil {
		t.Fatalf("add to cart B: %v", err)
	}

	if err := testStore.ReserveCartForCheckout(int(userA), "expired-key", time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("reserve expired cart: %v", err)
	}
	if err := testStore.ReserveCartForCheckout(int(userB), "fresh-key", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("reserve fresh cart: %v", err)
	}

	reservations, err := testStore.ListActiveStockReservationsByKey(int(userA), "expired-key")
	if err != nil {
		t.Fatalf("list expired reservations: %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("expected 0 active reservations for expired key, got %d", len(reservations))
	}

	reservations, err = testStore.ListActiveStockReservationsByKey(int(userB), "fresh-key")
	if err != nil {
		t.Fatalf("list fresh reservations: %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("expected 1 active reservation for fresh key, got %d", len(reservations))
	}
}

func TestUpsertCheckoutAndPlaceOrderFromCheckout(t *testing.T) {
	suffix := time.Now().UnixNano()
	userID, err := testStore.CreateUser(User{
		Username:     fmt.Sprintf("checkout_user_%d", suffix),
		Email:        fmt.Sprintf("checkout_user_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("checkout-user-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	categoryID, err := testStore.CreateCategory(Category{
		Name:        fmt.Sprintf("Checkout Category %d", suffix),
		Slug:        fmt.Sprintf("checkout-category-%d", suffix),
		Description: "checkout",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID, err := testStore.CreateProduct(Product{
		Name:          "Checkout Product",
		Slug:          fmt.Sprintf("checkout-product-%d", suffix),
		Description:   "checkout product",
		Price:         10,
		StockQuantity: 4,
		CategoryID:    sqlNullInt64(categoryID),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := testStore.AddToCart(int(userID), int(productID), 2); err != nil {
		t.Fatalf("add to cart: %v", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	if err := testStore.ReserveCartForCheckout(int(userID), "checkout-key", expiresAt); err != nil {
		t.Fatalf("reserve cart: %v", err)
	}

	checkout, err := testStore.UpsertCheckout(int(userID), CheckoutInput{
		Token:          "checkout-key",
		Currency:       "USD",
		PaymentMethod:  "cash_on_delivery",
		SubtotalAmount: 20,
		ShippingFee:    6.50,
		TaxAmount:      1.60,
		TotalAmount:    28.10,
		ExpiresAt:      expiresAt,
		Lines: []CheckoutLineInput{
			{
				ProductID:      int(productID),
				ProductName:    "Checkout Product",
				Quantity:       2,
				UnitPrice:      10,
				LineTotal:      20,
				ReservationKey: "checkout-key",
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert checkout: %v", err)
	}
	if checkout == nil {
		t.Fatal("expected checkout")
	}
	if checkout.Status != CheckoutStatusOpen {
		t.Fatalf("checkout status = %q, want %q", checkout.Status, CheckoutStatusOpen)
	}
	if len(checkout.Lines) != 1 {
		t.Fatalf("checkout lines length = %d, want 1", len(checkout.Lines))
	}

	if err := testStore.UpdateCartQuantity(int(userID), int(productID), 1); err != nil {
		t.Fatalf("update cart quantity: %v", err)
	}

	placement, err := testStore.PlaceOrderFromCheckoutWithPayment(int(userID), "checkout-key", "Snapshot Lane", "Checkout snapshot test", PaymentRecordInput{
		Method:            "card_placeholder",
		Provider:          "placeholder_card",
		Status:            PaymentStatusCaptured,
		Currency:          "USD",
		Amount:            0,
		ExternalReference: "pay_snapshot",
	})
	if err != nil {
		t.Fatalf("place order from checkout: %v", err)
	}
	if placement.OrderID == 0 {
		t.Fatal("expected non-zero order id")
	}
	if math.Abs(placement.TotalAmount-28.10) > 0.001 {
		t.Fatalf("placement total = %.2f, want 28.10", placement.TotalAmount)
	}

	orders, err := testStore.ListOrdersByUser(int(userID))
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	if len(orders[0].Items) != 1 {
		t.Fatalf("expected 1 order item, got %d", len(orders[0].Items))
	}
	if orders[0].Items[0].Quantity != 2 {
		t.Fatalf("order item quantity = %d, want 2", orders[0].Items[0].Quantity)
	}

	checkout, err = testStore.GetCheckoutByToken(int(userID), "checkout-key")
	if err != nil {
		t.Fatalf("get checkout after placement: %v", err)
	}
	if checkout == nil {
		t.Fatal("expected checkout after placement")
	}
	if checkout.Status != CheckoutStatusCompleted {
		t.Fatalf("checkout status = %q, want %q", checkout.Status, CheckoutStatusCompleted)
	}
	if !checkout.OrderID.Valid || checkout.OrderID.Int64 != placement.OrderID {
		t.Fatalf("checkout order id = %v, want %d", checkout.OrderID, placement.OrderID)
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
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestUpdateOrderByAdmin_OrderNotFound(t *testing.T) {
	err := testStore.UpdateOrderByAdmin(999999, "pending", "new", "queued", "no-op")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
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

func TestUsePasswordResetToken_InvalidatesUserSessions(t *testing.T) {
	suffix := time.Now().UnixNano()
	localDBPath := fmt.Sprintf("./test_reset_%d.db", suffix)
	localStore, err := OpenStore(localDBPath)
	if err != nil {
		t.Fatalf("open local store: %v", err)
	}
	if err := localStore.ApplySchema(); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() {
		_ = localStore.Close()
		_ = os.Remove(localDBPath)
	})

	userID, err := localStore.CreateUser(User{
		Username:     fmt.Sprintf("reset_target_%d", suffix),
		Email:        fmt.Sprintf("reset_target_%d@example.com", suffix),
		PasswordHash: "oldhash",
		Salt:         "oldsalt",
		Role:         "user",
		Slug:         fmt.Sprintf("reset-target-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}

	otherUserID, err := localStore.CreateUser(User{
		Username:     fmt.Sprintf("reset_other_%d", suffix),
		Email:        fmt.Sprintf("reset_other_%d@example.com", suffix),
		PasswordHash: "otherhash",
		Salt:         "othersalt",
		Role:         "user",
		Slug:         fmt.Sprintf("reset-other-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	sessionA := fmt.Sprintf("session_a_%d", suffix)
	sessionB := fmt.Sprintf("session_b_%d", suffix)
	otherSession := fmt.Sprintf("other_session_%d", suffix)

	if err := localStore.CreateSession(Session{ID: sessionA, UserID: int(userID), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session A: %v", err)
	}
	if err := localStore.CreateSession(Session{ID: sessionB, UserID: int(userID), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session B: %v", err)
	}
	if err := localStore.CreateSession(Session{ID: otherSession, UserID: int(otherUserID), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create other session: %v", err)
	}

	resetToken := fmt.Sprintf("reset_token_%d", suffix)
	if err := localStore.CreatePasswordResetToken(int(userID), resetToken, time.Now().Add(30*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	used, err := localStore.UsePasswordResetToken(resetToken, "newhash", "newsalt")
	if err != nil {
		t.Fatalf("use reset token: %v", err)
	}
	if !used {
		t.Fatal("expected reset token usage to succeed")
	}

	if sess, err := localStore.GetSession(sessionA); err != nil {
		t.Fatalf("lookup session A: %v", err)
	} else if sess != nil {
		t.Fatal("expected session A to be invalidated")
	}
	if sess, err := localStore.GetSession(sessionB); err != nil {
		t.Fatalf("lookup session B: %v", err)
	} else if sess != nil {
		t.Fatal("expected session B to be invalidated")
	}

	if sess, err := localStore.GetSession(otherSession); err != nil {
		t.Fatalf("lookup other session: %v", err)
	} else if sess == nil {
		t.Fatal("expected unrelated user session to remain active")
	}

	if token, err := localStore.GetValidPasswordResetToken(resetToken); err != nil {
		t.Fatalf("lookup reset token after use: %v", err)
	} else if token != nil {
		t.Fatal("expected used reset token to be invalid")
	}
}

func sqlNullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}
