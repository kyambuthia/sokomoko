package db

import (
	"os"
	"testing"
	"database/sql"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const testDBPath = "./test_t.db"

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
	DB, err = sql.Open("sqlite3", testDBPath)
	if err != nil {
		panic(err)
	}

	if err = DB.Ping(); err != nil {
		panic(err)
	}

	// Apply schema
		_, err = DB.Exec(SchemaSQL)
	if err != nil {
		panic(err)
	}
}

func teardownTestDB() {
	if DB != nil {
		DB.Close()
	}
	os.Remove(testDBPath)
}

func TestInitDB(t *testing.T) {
	// InitDB is called in TestMain, so we just need to ensure DB is not nil
	if DB == nil {
		t.Error("DB connection is nil after InitDB")
	}

	// Verify tables exist
	rows, err := DB.Query("SELECT name FROM sqlite_master WHERE type='table';")
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

	id, err := CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero ID for new user, got 0")
	}

	// Verify user exists in DB
	retrievedUser, err := GetUserByID(int(id))
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
	_, err := CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for test: %v", err)
	}

	foundUser, err := GetUserByUsername(username)
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
	notFoundUser, err := GetUserByUsername("nonexistent")
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
	id, err := CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for test: %v", err)
	}

	foundUser, err := GetUserByID(int(id))
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
	notFoundUser, err := GetUserByID(99999)
	if err != nil {
		t.Fatalf("GetUserByID for non-existent user failed: %v", err)
	}
	if notFoundUser != nil {
		t.Error("Expected nil for non-existent user, got a user")
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
	id, err := CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for update test: %v", err)
	}

	user.ID = int(id)
	user.Email = "updated@example.com"
	user.Role = "admin"

	err = UpdateUser(user)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	updatedUser, err := GetUserByID(int(id))
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
	id, err := CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user for delete test: %v", err)
	}

	err = DeleteUser(int(id))
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	deletedUser, err := GetUserByID(int(id))
	if err != nil {
		t.Fatalf("GetUserByID after delete failed: %v", err)
	}

	if deletedUser != nil {
		t.Error("User found after deletion, expected nil")
	}
}