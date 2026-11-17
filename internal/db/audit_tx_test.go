package db

import "testing"

func TestAuditInsertFailureRollsBackUserChange(t *testing.T) {
	s := newTestStore(t)
	adminID := mustCreateUser(t, s, RoleAdmin)
	targetID := mustCreateUser(t, s, RoleStaff)

	if _, err := s.DB.Exec(`
CREATE FUNCTION test_fail_audit() RETURNS trigger
LANGUAGE plpgsql AS $fn$
BEGIN
  RAISE EXCEPTION 'audit insert failed';
END
$fn$`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER test_fail_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_fail_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.DB.Exec(`DROP TRIGGER IF EXISTS test_fail_audit ON audit_logs`)
		_, _ = s.DB.Exec(`DROP FUNCTION IF EXISTS test_fail_audit()`)
	})

	ctx := WithAudit(t.Context(), adminID, "user.deactivate", "user", targetID, "deactivated via admin team")
	if err := s.DeleteUserForActor(ctx, adminID, targetID); err == nil {
		t.Fatal("deactivation committed even though the audit insert failed")
	}

	var active bool
	if err := s.DB.QueryRow(`SELECT deleted_at IS NULL FROM users WHERE id = $1`, targetID).Scan(&active); err != nil || !active {
		t.Fatalf("user active=%t err=%v", active, err)
	}
	var audits int
	if err := s.DB.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'user.deactivate' AND target_id = $1`, targetID).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("audit rows = %d, %v", audits, err)
	}
}
