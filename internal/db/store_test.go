package db

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestApplySchema_IsIdempotentAndRecordsVersion(t *testing.T) {
	s := newTestStore(t)
	if err := s.ApplySchema(); err != nil {
		t.Fatalf("second ApplySchema: %v", err)
	}
	version, err := s.SchemaVersion(t.Context())
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

	id, err := s.CreateUser(t.Context(), User{Username: "Alice", Email: "Alice@Example.com", PasswordHash: "h", Role: RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	byName, err := s.GetUserByUsername(t.Context(), "alice")
	if err != nil || byName == nil || byName.ID != int(id) {
		t.Fatalf("lookup by lower-case username = %+v, %v", byName, err)
	}
	if byName.Email != "alice@example.com" {
		t.Fatalf("email stored as %q, want lower-case", byName.Email)
	}
	byEmail, err := s.GetUserByEmail(t.Context(), "ALICE@example.COM")
	if err != nil || byEmail == nil || byEmail.ID != int(id) {
		t.Fatalf("lookup by email = %+v, %v", byEmail, err)
	}

	_, err = s.CreateUser(t.Context(), User{Username: "ALICE", Email: "other@example.com", PasswordHash: "h"})
	if !IsUniqueConstraintError(err) {
		t.Fatalf("duplicate username error = %v, want unique violation", err)
	}
	_, err = s.CreateUser(t.Context(), User{Username: "bob", Email: "alice@EXAMPLE.com", PasswordHash: "h"})
	if !IsUniqueConstraintError(err) {
		t.Fatalf("duplicate email error = %v, want unique violation", err)
	}
}

func TestUsers_UpdateAndSoftDelete(t *testing.T) {
	s := newTestStore(t)
	id := mustCreateUser(t, s, RoleStaff)

	user, _ := s.GetUserByID(t.Context(), id)
	user.Email = "renamed@example.com"
	if err := s.UpdateUser(t.Context(), *user); err != nil {
		t.Fatalf("update user: %v", err)
	}
	updated, _ := s.GetUserByID(t.Context(), id)
	if updated.Email != "renamed@example.com" {
		t.Fatalf("email = %q", updated.Email)
	}

	if err := s.CreateSession(t.Context(), Session{ID: "tok-" + nextSuffix(), UserID: id, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.DeleteUser(t.Context(), id); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if deleted, _ := s.GetUserByID(t.Context(), id); deleted != nil {
		t.Fatalf("deleted user still visible")
	}
	if count, _ := s.CountActiveSessions(t.Context()); count != 0 {
		t.Fatalf("sessions after delete = %d, want 0", count)
	}
}

func TestSessions_StoredHashedAndExpire(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)

	if err := s.CreateSession(t.Context(), Session{ID: "raw-token", UserID: userID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess, err := s.GetSession(t.Context(), "raw-token")
	if err != nil || sess == nil {
		t.Fatalf("get session: %v %v", sess, err)
	}
	if sess.ID == "raw-token" {
		t.Fatalf("session id stored in plain text")
	}
	if again, _ := s.GetSession(t.Context(), sess.ID); again != nil {
		t.Fatalf("lookup by stored hash must not succeed")
	}

	if err := s.CreateSession(t.Context(), Session{ID: "old-token", UserID: userID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if expired, _ := s.GetSession(t.Context(), "old-token"); expired != nil {
		t.Fatalf("expired session returned")
	}
	if err := s.CleanupSessions(t.Context()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if count, _ := s.CountActiveSessions(t.Context()); count != 1 {
		t.Fatalf("active sessions = %d, want 1", count)
	}
}

func TestPasswordReset_SingleUseAndRevokesSessions(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	_ = s.CreateSession(t.Context(), Session{ID: "sess", UserID: userID, CSRFToken: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := s.CreatePasswordResetToken(t.Context(), userID, "first", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := s.CreatePasswordResetToken(t.Context(), userID, "second", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok, _ := s.GetValidPasswordResetToken(t.Context(), "first"); tok != nil {
		t.Fatalf("earlier token should be invalidated")
	}

	used, err := s.UsePasswordResetToken(t.Context(), "second", "new-hash")
	if err != nil || !used {
		t.Fatalf("use token = %v, %v", used, err)
	}
	if user, _ := s.GetUserByID(t.Context(), userID); user.PasswordHash != "new-hash" {
		t.Fatalf("password hash not updated")
	}
	if sess, _ := s.GetSession(t.Context(), "sess"); sess != nil {
		t.Fatalf("sessions should be revoked after reset")
	}
	if used, _ := s.UsePasswordResetToken(t.Context(), "second", "again"); used {
		t.Fatalf("token reused")
	}

	_ = s.CreatePasswordResetToken(t.Context(), userID, "expired", time.Now().Add(-time.Minute))
	if used, _ := s.UsePasswordResetToken(t.Context(), "expired", "x"); used {
		t.Fatalf("expired token accepted")
	}
}

func TestUpdateUserPassword_RevokesSessions(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	_ = s.CreateSession(t.Context(), Session{ID: "sess", UserID: userID, CSRFToken: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := s.UpdateUserPassword(t.Context(), userID, "rotated"); err != nil {
		t.Fatalf("update password: %v", err)
	}
	if sess, _ := s.GetSession(t.Context(), "sess"); sess != nil {
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

func TestBootstrapAdmin_IsAtomicAndSingleShot(t *testing.T) {
	s := newTestStore(t)
	// A pre-existing user occupies the first staff username.
	if _, err := s.CreateUser(t.Context(), User{Username: "staff01", Email: "taken@example.com", PasswordHash: "h"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	namer := func(i, attempt int) (string, string) {
		name := fmt.Sprintf("staff%02d", i)
		if attempt > 0 {
			name = fmt.Sprintf("staff%02d_%d", i, attempt)
		}
		return name, name + "@sokomoko.local"
	}

	staff, err := s.BootstrapAdmin(t.Context(), User{Username: "root", Email: "root@example.com", PasswordHash: "h"}, []string{"h1", "h2"}, namer)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if len(staff) != 2 || staff[0].Username != "staff01_1" || staff[1].Username != "staff02" {
		t.Fatalf("staff = %+v", staff)
	}

	if _, err := s.BootstrapAdmin(t.Context(), User{Username: "root2", Email: "root2@example.com", PasswordHash: "h"}, nil, namer); !errors.Is(err, ErrAdminExists) {
		t.Fatalf("second bootstrap err = %v, want ErrAdminExists", err)
	}
	if count, _ := s.CountUsersByRole(t.Context(), RoleAdmin); count != 1 {
		t.Fatalf("admins = %d", count)
	}
}

func TestBootstrapAdmin_ConcurrentCallsCreateOneAdmin(t *testing.T) {
	s := newTestStore(t)
	namer := func(i, attempt int) (string, string) {
		name := fmt.Sprintf("staff%02d_%d", i, attempt)
		return name, name + "@sokomoko.local"
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.BootstrapAdmin(t.Context(), User{Username: fmt.Sprintf("root%d", i), Email: fmt.Sprintf("root%d@example.com", i), PasswordHash: "h"}, []string{"h"}, namer)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAdminExists):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful bootstraps = %d, want 1", successes)
	}
	if count, _ := s.CountUsersByRole(t.Context(), RoleStaff); count != 1 {
		t.Fatalf("staff = %d, want 1 (no partial provisioning from losers)", count)
	}
}

func TestDashboardCounts_MatchesIndividualCounts(t *testing.T) {
	s := newTestStore(t)
	roles := []string{RoleAdmin, RoleStaff, RoleStaff, RoleUser, RoleUser, RoleUser, RolePartner}
	var lastUser int64
	for i, role := range roles {
		id, err := s.CreateUser(t.Context(), User{Username: fmt.Sprintf("dash%d", i), Email: fmt.Sprintf("dash%d@example.com", i), PasswordHash: "h", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		lastUser = id
	}
	if err := s.CreateSession(t.Context(), Session{ID: "dash-session", UserID: int(lastUser), CSRFToken: "c", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// A deactivated customer drops out of every count.
	deleted, err := s.CreateUser(t.Context(), User{Username: "gone", Email: "gone@example.com", PasswordHash: "h", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(t.Context(), int(deleted)); err != nil {
		t.Fatal(err)
	}

	got, err := s.DashboardCounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	products, _ := s.CountProducts(t.Context())
	sessions, _ := s.CountActiveSessions(t.Context())
	want := DashboardCounts{Products: products, Sessions: sessions, Admins: 1, Staff: 2, Users: 3}
	if got != want || sessions != 1 {
		t.Fatalf("DashboardCounts = %+v, want %+v", got, want)
	}
}

func TestRunExclusive_SecondHolderSkips(t *testing.T) {
	s := newTestStore(t)
	const key int64 = 0x7465737478636c75
	inside := make(chan struct{})
	leave := make(chan struct{})
	done := make(chan bool)
	go func() {
		ran, err := s.RunExclusive(t.Context(), key, func(context.Context) {
			close(inside)
			<-leave
		})
		if err != nil {
			t.Error(err)
		}
		done <- ran
	}()
	<-inside
	ran, err := s.RunExclusive(t.Context(), key, func(context.Context) { t.Error("second holder ran") })
	if err != nil || ran {
		t.Fatalf("second RunExclusive = %v, %v; want skipped", ran, err)
	}
	close(leave)
	if !<-done {
		t.Fatal("first holder did not run")
	}
	// The lock is released afterwards.
	ran, err = s.RunExclusive(t.Context(), key, func(context.Context) {})
	if err != nil || !ran {
		t.Fatalf("RunExclusive after release = %v, %v", ran, err)
	}
}
