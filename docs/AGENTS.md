# AGENTS.md - Development Guide for Sokomoko E-Commerce Platform

This document provides essential information for AI agents working on the Sokomoko e-commerce platform built with Go, HTML, CSS, JavaScript, and SQLite3.

## Build Commands

### Running the Application
```bash
# Apply schema only
go run ./cmd/sokomoko migrate

# Apply schema and seed bootstrap data
go run ./cmd/sokomoko seed

# Main application - default port 6969
go run ./cmd/sokomoko serve

# One-step local boot with bootstrap data
go run ./cmd/sokomoko serve --seed
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/db
go test ./internal/auth
go test ./cmd/sokomoko

# Run single test
go test ./internal/db -run TestCreateUser
go test ./internal/auth -run TestSignUp

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
├── cmd/
│   └── sokomoko/            # CLI entrypoint and startup commands
├── internal/
│   ├── app/                 # App composition and rendering
│   ├── auth/                # Authentication
│   ├── bootstrap/           # Schema/seed orchestration
│   ├── config/              # Environment config loading
│   ├── db/                  # Database layer
│   ├── routes/              # HTTP handlers
│   ├── service/             # Feature services and adapters
│   └── ui/                  # HTML templates and static assets
├── db/schema.sql            # Database schema
└── go.mod                   # Go module
```

## Code Style Guidelines

## UI Design Ethos (Mandatory)

The UI direction is paper-clean, structured, and quiet. Every change to templates, CSS, and JS should reinforce this.

### Design Tenets
- Plain paper baseline: white sheet surfaces, subtle borders, minimal shadows, and low-saturation accents.
- Capsule controls: all primary interactive controls (`.btn`, nav links, toggles) should use rounded capsule radii.
- Top navigation only: navigation stays in the header area on desktop and mobile; avoid sidebar navigation.
- Mobile-first layout: default styles target small screens, then scale up with `min-width` media queries.
- Strong spacing rhythm: use tokenized spacing (`--space-*`) and avoid ad-hoc pixel spacing.
- BEM-first components: prefer `block__element--modifier` naming and keep selectors shallow.
- Local-first typography: use local/system monospace stacks only; never add hosted web fonts.
- Gentle hierarchy: rely on spacing, border contrast, and weight before adding visual effects.
- Progressive enhancement: core navigation and forms must work without JS; JS adds behavior only.
- Consistency over novelty: reuse existing component primitives before introducing new variants.

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
    
    "github.com/kyambuthia/sokomoko/internal/db"
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

The active application uses the schema applied by the current `internal/db` store layer and `db/schema.sql`.

When making database changes:
- Update `db/schema.sql`
- Keep the `internal/db` query layer in sync
- Run `go run ./cmd/sokomoko migrate` or `go test ./...` to verify behavior

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

- `PORT`: Server port (default: 6969)
- `DB_PATH`: Database file path (default: ./db/t.db)
- `ALLOWED_HOSTS`: Comma-separated trusted hosts
- `ADMIN_SETUP_TOKEN`: Optional token for admin bootstrap outside loopback
- `SESSION_COOKIE_DOMAIN`: Optional shared cookie domain
- `PASSWORD_RESET_BASE_URL`: Optional absolute base URL for reset links
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`: Optional SMTP settings for password reset delivery

## Development Notes

- Use `serve`, `migrate`, and `seed` commands via `./cmd/sokomoko`
- Admin bootstrap lives at `admin.localhost/setup`
- Static files embedded using Go's embed directive
