# Authentication System

## Overview

The Sokomoko e-commerce platform implements a comprehensive authentication system that supports both user and admin authentication. The system uses session-based authentication with secure cookie storage.

## Design

### Authentication Flow

1. **User Registration** (`POST /signup`)
   - User provides username, email, and password
   - Server generates a random salt
   - Password is hashed using bcrypt with the salt
   - User is stored in the database with default role "user"
   - User is redirected to login page

2. **User Login** (`POST /login`)
   - User provides username and password
   - Server retrieves user by username
   - Password is verified by comparing hashed password with `password + salt`
   - On success, a secure session token is generated
   - Session is stored in database with expiration time (24 hours)
   - Session cookie is set with HttpOnly flag

3. **Admin Login** (`POST /login` on admin.localhost)
   - Same flow as user login
   - Additional check verifies user has "admin" role
   - Non-admin users receive 401 Unauthorized

4. **Logout** (`POST /logout`)
   - Session is deleted from database
   - Cookie is cleared (expired)
   - User is redirected to home page

### Security Features

- **Password Hashing**: bcrypt with random salt per user
- **Session Management**: Secure random session tokens
- **Cookie Security**: HttpOnly flag to prevent XSS
- **Role-Based Access Control**: Separate access for admins and users
- **Session Expiration**: 24-hour session validity
- **Input Validation**: All fields are required and validated

### Database Schema

#### Users Table
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    slug TEXT UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
```

#### Sessions Table
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

## API Endpoints

### Public Routes

| Method | Path | Description | Auth Required |
|--------|------|-------------|---------------|
| GET | `/login` | Display login form | No |
| POST | `/login` | Authenticate user | No |
| GET | `/signup` | Display registration form | No |
| POST | `/signup` | Register new user | No |
| POST | `/logout` | Logout user | Yes |

### Admin Routes

| Method | Path | Description | Auth Required |
|--------|------|-------------|---------------|
| GET | `/login` | Display admin login form | No |
| POST | `/login` | Authenticate admin | No |
| GET | `/` | Admin dashboard | Admin role |
| GET | `/products` | Product management | Admin role |
| GET | `/orders` | Order management | Admin role |
| GET | `/reports` | Sales reports | Admin role |
| GET | `/deliveries` | Delivery management | Admin role |

## Middleware

### AuthMiddleware
Verifies session cookie and adds user to request context:
- Redirects to `/login` if no valid session
- Checks session expiration
- Loads user from database

### RequireRole
Checks authenticated user has required role:
- Returns 403 Forbidden if role doesn't match
- Used with AuthMiddleware for protected routes

## Default Admin User

The system seeds a default admin user on first run:
- **Username**: `admin`
- **Password**: `adminpass`
- **Email**: `admin@sokomoko.com`

**Important**: Change the default admin password in production!

## Testing Authentication

### Using the Test Script

A bash script is provided for manual testing with curl:

```bash
cd cmd/sokomoko
./test_auth.sh
```

The script requires the server to be running and tests:
1. GET login/signup pages
2. User registration
3. User login
4. Session verification
5. Logout
6. Invalid login attempts
7. Admin login and dashboard access

### Using Go Tests

Run the integration tests:

```bash
go test -v ./cmd/sokomoko/...
go test -v ./internal/auth/...
```

### Manual Testing with curl

Start the server:
```bash
go run cmd/sokomoko/main.go
```

Test user signup:
```bash
curl -c cookies.txt -X POST \
  -d "username=testuser" \
  -d "email=test@example.com" \
  -d "password=testpass123" \
  http://localhost:6969/signup
```

Test user login:
```bash
curl -c cookies.txt -X POST \
  -d "username=testuser" \
  -d "password=testpass123" \
  http://localhost:6969/login
```

Access protected route:
```bash
curl -b cookies.txt http://localhost:6969/account
```

Test admin login (requires admin subdomain):
```bash
curl -c admin_cookies.txt -X POST \
  -d "username=admin" \
  -d "password=adminpass" \
  -H "Host: admin.localhost" \
  http://localhost:6969/login
```

Access admin dashboard:
```bash
curl -b admin_cookies.txt -H "Host: admin.localhost" \
  http://localhost:6969/
```

Logout:
```bash
curl -b cookies.txt -c cookies.txt -X POST \
  http://localhost:6969/logout
```

## Server Graceful Shutdown

The server supports graceful shutdown via SIGINT (Ctrl+C) or SIGTERM:

1. Signal is received
2. Server stops accepting new connections
3. Active connections are handled with 30-second timeout
4. Database connection is closed
5. Port is released

Implementation uses Go's `context`, `os/signal`, and `http.Server.Shutdown`:

```go
go func() {
    sigint := make(chan os.Signal, 1)
    signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
    <-sigint

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    srv.Shutdown(ctx)
}()
```

## File Structure

```
internal/
├── auth/
│   ├── auth.go          # Auth handlers and middleware
│   └── auth_test.go     # Unit tests
├── routes/
│   ├── register.go      # Route registration
│   └── admin.go         # Admin routes
├── db/
│   └── db.go           # Database layer
└── ui/
    └── templates.go     # Template parsing

cmd/sokomoko/
├── main.go             # Server entry point with graceful shutdown
├── test_auth.sh        # Bash script for manual testing
└── integration_test.go  # Integration tests
```

## Security Considerations

1. **Password Storage**: Never store plain text passwords
2. **Session Tokens**: Use cryptographically secure random generation
3. **Cookie Flags**: Always use HttpOnly for session cookies
4. **HTTPS**: Use Secure cookie flag in production (HTTPS required)
5. **Rate Limiting**: Consider implementing rate limiting for login attempts
6. **CSRF Protection**: Add CSRF tokens for forms (future enhancement)
7. **Password Complexity**: Enforce password policies (future enhancement)

## Future Enhancements

- [ ] Password reset functionality
- [ ] Email verification for new accounts
- [ ] Two-factor authentication
- [ ] CSRF token protection
- [ ] Rate limiting on login attempts
- [ ] Password complexity requirements
- [ ] OAuth 2.0 integration (Google, GitHub, etc.)
- [ ] Remember me functionality with extended sessions
