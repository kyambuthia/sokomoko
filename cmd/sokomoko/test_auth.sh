#!/bin/bash

# Integration test script for authentication flows using curl
# This script tests user and admin authentication endpoints

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Server configuration
BASE_URL="${BASE_URL:-http://localhost:6969}"
ADMIN_URL="${ADMIN_URL:-http://admin.localhost:6969}"
COOKIE_JAR="/tmp/sokomoko_cookies.txt"

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_failure() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

cleanup() {
    rm -f "$COOKIE_JAR"
}

cleanup

# Setup: Check if server is running
log_info "Checking if server is running at $BASE_URL..."
if ! curl -s --max-time 5 "$BASE_URL/" > /dev/null 2>&1; then
    echo "Error: Server is not running at $BASE_URL"
    echo "Please start the server with: go run ./cmd/sokomoko serve --seed"
    exit 1
fi
log_success "Server is running"

# Test 1: GET login page
log_info "Test 1: GET /login"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/login")
if [ "$STATUS" -eq 200 ]; then
    log_success "GET /login returned 200"
else
    log_failure "GET /login returned $STATUS (expected 200)"
fi

# Test 2: GET signup page
log_info "Test 2: GET /signup"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/signup")
if [ "$STATUS" -eq 200 ]; then
    log_success "GET /signup returned 200"
else
    log_failure "GET /signup returned $STATUS (expected 200)"
fi

# Test 3: Create a new user (signup)
log_info "Test 3: POST /signup - Create new user"
TIMESTAMP=$(date +%s)
USERNAME="testuser_$TIMESTAMP"
EMAIL="test_$TIMESTAMP@example.com"
PASSWORD="testpass123"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -d "username=$USERNAME" \
    -d "email=$EMAIL" \
    -d "password=$PASSWORD" \
    "$BASE_URL/signup")

if [ "$STATUS" -eq 302 ]; then
    LOCATION=$(curl -s -i -X POST \
        -d "username=$USERNAME" \
        -d "email=$EMAIL" \
        -d "password=$PASSWORD" \
        "$BASE_URL/signup" 2>/dev/null | grep -i "Location:" | cut -d' ' -f2 | tr -d '\r')

    if [ "$LOCATION" = "/login" ]; then
        log_success "Signup successful - redirected to /login"
    else
        log_failure "Signup redirected to $LOCATION (expected /login)"
    fi
else
    log_failure "POST /signup returned $STATUS (expected 302)"
fi

# Test 4: Login with the created user
log_info "Test 4: POST /login - Login with created user"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -d "username=$USERNAME" \
    -d "password=$PASSWORD" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 302 ]; then
    LOCATION=$(curl -s -i -X POST \
        -d "username=$USERNAME" \
        -d "password=$PASSWORD" \
        "$BASE_URL/login" 2>/dev/null | grep -i "Location:" | cut -d' ' -f2 | tr -d '\r')

    if [ "$LOCATION" = "/" ]; then
        log_success "Login successful - redirected to /"
    else
        log_failure "Login redirected to $LOCATION (expected /)"
    fi
else
    log_failure "POST /login returned $STATUS (expected 302)"
fi

# Test 5: Verify session cookie is set
log_info "Test 5: Verify session cookie is set"
if grep -q "session_token" "$COOKIE_JAR"; then
    log_success "Session cookie is set"
else
    log_failure "Session cookie not found"
fi

# Test 6: Access protected route (logout)
log_info "Test 6: POST /logout - Logout"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -b "$COOKIE_JAR" \
    -c "$COOKIE_JAR" \
    -X POST \
    "$BASE_URL/logout")

if [ "$STATUS" -eq 302 ]; then
    log_success "Logout successful"
else
    log_failure "POST /logout returned $STATUS (expected 302)"
fi

# Test 7: Verify session is cleared after logout
log_info "Test 7: Verify session is cleared after logout"
COOKIE_VALUE=$(grep "session_token" "$COOKIE_JAR" | awk '{print $7}')
if [ -z "$COOKIE_VALUE" ]; then
    log_success "Session cleared after logout"
else
    log_failure "Session still present after logout"
fi

# Test 8: Try to login with wrong password
log_info "Test 8: POST /login - Login with wrong password"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -d "username=$USERNAME" \
    -d "password=wrongpassword" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 401 ]; then
    log_success "Login failed with wrong password - returned 401"
else
    log_failure "Login with wrong password returned $STATUS (expected 401)"
fi

# Test 9: Try to login with non-existent user
log_info "Test 9: POST /login - Login with non-existent user"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -d "username=nonexistent_user_$TIMESTAMP" \
    -d "password=$PASSWORD" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 401 ]; then
    log_success "Login with non-existent user returned 401"
else
    log_failure "Login with non-existent user returned $STATUS (expected 401)"
fi

# Test 10: GET admin login page
log_info "Test 10: GET /login on admin subdomain"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Host: admin.localhost" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 200 ]; then
    log_success "Admin login page returned 200"
else
    log_failure "Admin login page returned $STATUS (expected 200)"
fi

# Test 11: Admin login
log_info "Test 11: POST /login - Admin login"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -H "Host: admin.localhost" \
    -d "username=admin" \
    -d "password=adminpass" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 302 ]; then
    LOCATION=$(curl -s -i -X POST \
        -H "Host: admin.localhost" \
        -d "username=admin" \
        -d "password=adminpass" \
        "$BASE_URL/login" 2>/dev/null | grep -i "Location:" | cut -d' ' -f2 | tr -d '\r')

    if [ "$LOCATION" = "/" ]; then
        log_success "Admin login successful - redirected to /"
    else
        log_failure "Admin login redirected to $LOCATION (expected /)"
    fi
else
    log_failure "Admin login returned $STATUS (expected 302)"
fi

# Test 12: Access admin dashboard with admin session
log_info "Test 12: GET / on admin subdomain - Admin dashboard"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -b "$COOKIE_JAR" \
    -H "Host: admin.localhost" \
    "$BASE_URL/")

if [ "$STATUS" -eq 200 ]; then
    log_success "Admin dashboard accessible with admin session - returned 200"
else
    log_failure "Admin dashboard returned $STATUS (expected 200)"
fi

# Test 13: Try to access admin dashboard with non-admin user
log_info "Test 13: POST /login - Regular user trying to access admin"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -c "$COOKIE_JAR" \
    -X POST \
    -H "Host: admin.localhost" \
    -d "username=$USERNAME" \
    -d "password=$PASSWORD" \
    "$BASE_URL/login")

if [ "$STATUS" -eq 401 ]; then
    log_success "Non-admin user cannot access admin login - returned 401"
else
    log_failure "Non-admin user returned $STATUS (expected 401)"
fi

# Print summary
echo ""
echo "==================================="
echo "Test Summary"
echo "==================================="
echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"
echo "==================================="

cleanup

if [ $TESTS_FAILED -gt 0 ]; then
    exit 1
fi

exit 0
