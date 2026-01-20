# AGENTS.md - Development Guide for Sokomoko E-Commerce Platform

This document provides essential information for AI agents working on the Sokomoko e-commerce platform built with Go, HTML, CSS, JavaScript, and SQLite3.

## Build Commands

### Running the Application
```bash
# Main application (new architecture) - default port 6969
go run src/main.go

# Legacy server - default port 8000
go run src/srvr.go
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./src/db
go test ./src/auth

# Run single test
go test ./src/db -run TestCreateUser
go test ./src/auth -run TestSignUp

# Verbose output and coverage
go test -v ./...
go test -cover ./...
```

### Dependencies
```bash
go mod tidy      # Install dependencies
go mod download  # Update dependencies
```

## Project Structure

```
sokomoko/
├── src/
│   ├── main.go              # Main application entry point
│   ├── srvr.go              # Legacy server
│   ├── db/                  # Database layer
│   ├── auth/                # Authentication
│   ├── routes/              # HTTP handlers
│   ├── templates/           # HTML templates
│   └── static/              # Static assets
├── db/schema.sql            # Database schema
└── go.mod                   # Go module
```

## Code Style Guidelines

### Go Code Style

#### Imports
- Group imports: standard library, third-party, local packages
- Use blank lines between groups, order alphabetically

```go
import (
    "database/sql"
    "log"
    "net/http"
    
    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"
    
    "github.com/kyambuthia/sokomoko/src/db"
)
```

#### Naming Conventions
- CamelCase for exported names, camelCase for private
- Package names: short, lowercase, single words
- Constants: UPPER_SNAKE_CASE or CamelCase if exported
- Function names: verb phrases

```go
type User struct {
    ID           int
    Username     string
    PasswordHash string `json:"-"`
}

func CreateUser(user User) (int64, error) { }
const DEFAULT_USER_ROLE = "user"
```

#### Error Handling
- Always handle errors explicitly, return as last value
- Use descriptive messages, log with context
- Never ignore errors with `_`

```go
user, err := GetUserByID(id)
if err != nil {
    log.Printf("Error retrieving user %d: %v", id, err)
    return nil, err
}
if user == nil {
    return nil, fmt.Errorf("user not found")
}
```

#### Database Operations
- Use prepared statements for parameters
- Always close statements and rows
- Handle nil results, use transactions for multi-step operations

#### HTTP Handlers
- Use http.HandlerFunc pattern, check methods explicitly
- Use proper HTTP status codes, templates for HTML responses

### Testing Guidelines

#### Test Structure
- Use TestMain for setup/teardown
- Name tests: TestFunctionName_Condition_ExpectedResult
- Clean up test data, test success and failure cases

```go
func TestCreateUser_ValidUser_ReturnsID(t *testing.T) {
    user := User{Username: "test", Email: "test@example.com"}
    id, err := CreateUser(user)
    if err != nil {
        t.Fatalf("CreateUser failed: %v", err)
    }
    if id == 0 {
        t.Error("Expected non-zero ID")
    }
}
```

### JavaScript Style
- Prefer const/let over var, use arrow functions
- Use template literals, async/await for async operations

## Database Schema

The project has two conflicting schemas in db/schema.sql:

**Working Schema (db.go)**: `users`, `categories`, `products` tables
**Alternative Schema (srvr.go)**: `Users`, `Products` with additional tables

**Always check which schema your code is using before making database changes.**

## Security Guidelines

### Password Handling
- Always use bcrypt for password hashing
- Generate unique salt for each user
- Never store plain text passwords
- Use secure password comparison

### Session Management
- Use secure, random session tokens
- Set appropriate cookie flags (HttpOnly, Secure in production)
- Implement session expiration
- Clean up expired sessions

### Input Validation
- Validate all user input
- Use parameterized queries to prevent SQL injection
- Sanitize HTML output to prevent XSS
- Implement CSRF protection for forms

## Common Patterns

### Middleware Pattern
Use middleware for authentication and other cross-cutting concerns.

### Template Data Structure
Use consistent template data structure across all handlers.

## Environment Variables

- `PORT`: Server port (default: 6969 for main.go, 8000 for srvr.go)
- `DB_PATH`: Database file path (default: ./db/t.db)

## Development Notes

- Two server implementations: main.go (new) and srvr.go (legacy)
- Database schema conflicts in db/schema.sql - resolve before changes
- Static files embedded using Go's embed directive
- No external frameworks beyond database drivers
- Testing uses SQLite in-memory databases for isolation