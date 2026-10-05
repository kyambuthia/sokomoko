package db

import (
	"errors"
	"testing"
	"time"
)

func TestApplySchema_IsIdempotentAndRecordsVersion(t *testing.T) {
	s := newTestStore(t)
	if err := s.ApplySchema(); err != nil {
		t.Fatalf("second ApplySchema: %v", err)
	}
	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != LatestSchemaVersion() || version == 0 {
		t.Fatalf("schema version = %d, want %d", version, LatestSchemaVersion())
	}

	for _, table := range []string{"users", "partners", "products", "inventory_stocks", "stock_reservations", "orders", "order_items", "checkouts", "payments", "audit_logs"} {
		var exists bool
		if err := s.DB.QueryRow("SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s missing (err=%v)", table, err)
		}
	}
}

func TestUsers_CaseInsensitiveIdentity(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateUser(User{Username: "Alice", Email: "Alice@Example.com", PasswordHash: "h", Role: RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	byName, err := s.GetUserByUsername("alice")
	if err != nil || byName == nil || byName.ID != int(id) {
		t.Fatalf("lookup by lower-case username = %+v, %v", byName, err)
	}
	if byName.Email != "alice@example.com" {
		t.Fatalf("email stored as %q, want lower-case", byName.Email)
	}
	byEmail, err := s.GetUserByEmail("ALICE@example.COM")
	if err != nil || byEmail == nil || byEmail.ID != int(id) {
		t.Fatalf("lookup by email = %+v, %v", byEmail, err)
	}

	_, err = s.CreateUser(User{Username: "ALICE", Email: "other@example.com", PasswordHash: "h"})
	if !IsUniqueConstraintError(err) {
		t.Fatalf("duplicate username error = %v, want unique violation", err)
	}
	_, err = s.CreateUser(User{Username: "bob", Email: "alice@EXAMPLE.com", PasswordHash: "h"})
	if !IsUniqueConstraintError(err) {
		t.Fatalf("duplicate email error = %v, want unique violation", err)
	}
}

func TestUsers_UpdateAndSoftDelete(t *testing.T) {
	s := newTestStore(t)
	id := mustCreateUser(t, s, RoleStaff)

	user, _ := s.GetUserByID(id)
	user.Email = "renamed@example.com"
	if err := s.UpdateUser(*user); err != nil {
		t.Fatalf("update user: %v", err)
	}
	updated, _ := s.GetUserByID(id)
	if updated.Email != "renamed@example.com" {
		t.Fatalf("email = %q", updated.Email)
	}

	if err := s.CreateSession(Session{ID: "tok-" + nextSuffix(), UserID: id, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.DeleteUser(id); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if deleted, _ := s.GetUserByID(id); deleted != nil {
		t.Fatalf("deleted user still visible")
	}
	if count, _ := s.CountActiveSessions(); count != 0 {
		t.Fatalf("sessions after delete = %d, want 0", count)
	}
}

func TestSessions_StoredHashedAndExpire(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)

	if err := s.CreateSession(Session{ID: "raw-token", UserID: userID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess, err := s.GetSession("raw-token")
	if err != nil || sess == nil {
		t.Fatalf("get session: %v %v", sess, err)
	}
	if sess.ID == "raw-token" {
		t.Fatalf("session id stored in plain text")
	}
	if again, _ := s.GetSession(sess.ID); again != nil {
		t.Fatalf("lookup by stored hash must not succeed")
	}

	if err := s.CreateSession(Session{ID: "old-token", UserID: userID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if expired, _ := s.GetSession("old-token"); expired != nil {
		t.Fatalf("expired session returned")
	}
	if err := s.CleanupSessions(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if count, _ := s.CountActiveSessions(); count != 1 {
		t.Fatalf("active sessions = %d, want 1", count)
	}
}

func TestPasswordReset_SingleUseAndRevokesSessions(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	_ = s.CreateSession(Session{ID: "sess", UserID: userID, CSRFToken: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := s.CreatePasswordResetToken(userID, "first", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := s.CreatePasswordResetToken(userID, "second", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok, _ := s.GetValidPasswordResetToken("first"); tok != nil {
		t.Fatalf("earlier token should be invalidated")
	}

	used, err := s.UsePasswordResetToken("second", "new-hash")
	if err != nil || !used {
		t.Fatalf("use token = %v, %v", used, err)
	}
	if user, _ := s.GetUserByID(userID); user.PasswordHash != "new-hash" {
		t.Fatalf("password hash not updated")
	}
	if sess, _ := s.GetSession("sess"); sess != nil {
		t.Fatalf("sessions should be revoked after reset")
	}
	if used, _ := s.UsePasswordResetToken("second", "again"); used {
		t.Fatalf("token reused")
	}

	_ = s.CreatePasswordResetToken(userID, "expired", time.Now().Add(-time.Minute))
	if used, _ := s.UsePasswordResetToken("expired", "x"); used {
		t.Fatalf("expired token accepted")
	}
}

func TestUpdateUserPassword_RevokesSessions(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	_ = s.CreateSession(Session{ID: "sess", UserID: userID, CSRFToken: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := s.UpdateUserPassword(userID, "rotated"); err != nil {
		t.Fatalf("update password: %v", err)
	}
	if sess, _ := s.GetSession("sess"); sess != nil {
		t.Fatalf("session survived password change")
	}
}

func TestWrapDBError_TransientClassification(t *testing.T) {
	if IsTransientError(errors.New("boom")) {
		t.Fatalf("plain error classified transient")
	}
	if !IsTransientError(ErrTransient) {
		t.Fatalf("ErrTransient not transient")
	}
}
