# Authentication System

Last updated: April 4, 2026

## Overview

Sokomoko uses cookie-backed sessions with host-specific login surfaces:

- `localhost`: customer signup, login, logout, password reset, and account access
- `admin.localhost`: initial admin bootstrap, admin or staff login, staff signup, password reset, and admin tools
- `partner.localhost`: admin or staff login, password reset, store setup, and partner operations

Authentication lives in [`internal/auth/auth.go`](/home/mbuthi/Projects/sokomoko/internal/auth/auth.go) and is registered through [`internal/routes/register.go`](/home/mbuthi/Projects/sokomoko/internal/routes/register.go).

## Current auth flows

### Customer signup

`POST /signup` on `localhost`

- validates username, email, and password
- hashes `password + salt` with bcrypt
- creates a `user` role account
- redirects to `/login`

### Customer login

`POST /login` on `localhost`

- looks up the user by username
- only allows role `user`
- creates a 24-hour session on success
- sets the `session_token` cookie

### Admin and staff login

`POST /login` on `admin.localhost` and `partner.localhost`

- checks that at least one admin account exists first
- looks up the user by username
- allows roles `admin` and `staff`
- creates a 24-hour session on success

### First-run admin bootstrap

`GET` or `POST /setup` on `admin.localhost`

- only available while no admin user exists
- allowed automatically from loopback requests
- otherwise requires `ADMIN_SETUP_TOKEN` via `X-Admin-Setup-Token` or `setup_token`
- creates the root admin account
- can also provision recommended staff accounts with generated temporary passwords

### Password reset

`/password-reset/request` and `/password-reset/confirm` exist on all three hosts.

- reset tokens are random, one-time, and expire after 45 minutes
- tokens are stored hashed in `password_reset_tokens`
- successful password reset invalidates existing sessions for that user
- in non-production environments the reset link can be rendered directly in the response
- in production the flow depends on SMTP-based email delivery

### Logout

`POST /logout`

- deletes the session from the database
- expires the cookie in the browser
- redirects back to `/`

## Session model

Sessions live in the `sessions` table.

- Cookie name: `session_token`
- Duration: 24 hours
- Cookie flags: `HttpOnly`, `SameSite=Lax`, `Secure` when HTTPS or production mode is detected
- Stored value: the cookie contains the opaque token, while the DB stores a SHA-256 hash of that token

## Middleware and authorization

### `AuthMiddleware`

- loads `session_token` from the cookie
- resolves the session from the DB
- loads the user record
- injects the user into request context
- redirects unauthenticated requests to `/login`

### `RequireRole`

- allows exactly one role
- used for admin-only routes such as staff signup, audit, and team actions

### `RequireAnyRole`

- allows either `admin` or `staff`
- used for the general admin and partner protected surfaces

## Current security posture

Implemented today:

- bcrypt password hashing with per-user salts
- hashed session IDs in storage
- hashed password-reset tokens in storage
- session invalidation on password reset
- host allowlist enforcement
- security headers
- request body limits
- same-origin checking for many unsafe requests
- optional IP-based POST rate limiting

Still missing and should be treated as active hardening work:

- token-based CSRF protection for auth and form posts
- auth-specific throttling and lockout behavior
- verified-email flow
- optional MFA for admin users
- stronger startup validation for production SMTP and cookie configuration

## Route summary

### Customer host: `localhost`

- `GET|POST /signup`
- `GET|POST /login`
- `POST /logout`
- `GET|POST /password-reset/request`
- `GET|POST /password-reset/confirm`
- `GET /account`

### Admin host: `admin.localhost`

- `GET|POST /setup`
- `GET|POST /login`
- `GET|POST /password-reset/request`
- `GET|POST /password-reset/confirm`
- `GET|POST /staff/signup`
- protected admin pages under `/`

### Partner host: `partner.localhost`

- `GET|POST /login`
- `GET|POST /password-reset/request`
- `GET|POST /password-reset/confirm`
- protected partner pages under `/setup`, `/dashboard`, `/products`, and `/orders`

## Relevant files

- [`internal/auth/auth.go`](/home/mbuthi/Projects/sokomoko/internal/auth/auth.go)
- [`internal/app/middleware.go`](/home/mbuthi/Projects/sokomoko/internal/app/middleware.go)
- [`internal/routes/register.go`](/home/mbuthi/Projects/sokomoko/internal/routes/register.go)
- [`internal/db/sessions.go`](/home/mbuthi/Projects/sokomoko/internal/db/sessions.go)
- [`db/schema.sql`](/home/mbuthi/Projects/sokomoko/db/schema.sql)

## Test commands

```bash
go test ./internal/auth
go test ./cmd/sokomoko
go test ./...
```
