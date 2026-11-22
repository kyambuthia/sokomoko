package db

import (
	"database/sql"
	"testing"
	"time"
)

func TestAPITokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	userID := mustCreateUser(t, s, RoleUser)
	otherUserID := mustCreateUser(t, s, RoleUser)

	raw1 := "smk_test_token_1"
	tok1, err := s.CreateAPIToken(ctx, userID, "ci-agent", "smk_test", raw1, "", sql.NullTime{})
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if tok1.UserID != userID || tok1.Name != "ci-agent" || tok1.ClientType != "agent" {
		t.Fatalf("unexpected token: %+v", tok1)
	}

	raw2 := "smk_test_token_2"
	exp := sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
	tok2, err := s.CreateAPIToken(ctx, userID, "cli", "smk_test", raw2, "cli", exp)
	if err != nil {
		t.Fatalf("CreateAPIToken 2: %v", err)
	}

	list, err := s.ListAPITokensByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListAPITokensByUser: %v", err)
	}
	if len(list) != 2 || list[0].ID != tok2.ID || list[1].ID != tok1.ID {
		t.Fatalf("unexpected list: %+v", list)
	}

	u, lookedUp, err := s.LookupActiveAPITokenUser(ctx, raw1)
	if err != nil || u == nil || lookedUp == nil {
		t.Fatalf("LookupActiveAPITokenUser failed: user=%v tok=%v err=%v", u, lookedUp, err)
	}
	if u.ID != userID || lookedUp.ID != tok1.ID {
		t.Fatalf("unexpected lookup result: user=%d tok=%d", u.ID, lookedUp.ID)
	}

	if err := s.TouchAPITokenLastUsed(ctx, tok1.ID); err != nil {
		t.Fatalf("TouchAPITokenLastUsed: %v", err)
	}
	listAfterTouch, err := s.ListAPITokensByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListAPITokensByUser after touch: %v", err)
	}
	if !listAfterTouch[1].LastUsedAt.Valid {
		t.Fatalf("expected LastUsedAt to be set")
	}

	// IDOR check: another user cannot revoke tok1.
	revoked, err := s.RevokeAPIToken(ctx, otherUserID, tok1.ID)
	if err != nil || revoked {
		t.Fatalf("expected other user revoke to return false, got revoked=%v err=%v", revoked, err)
	}

	// Owner can revoke tok1.
	revoked, err = s.RevokeAPIToken(ctx, userID, tok1.ID)
	if err != nil || !revoked {
		t.Fatalf("expected owner revoke to succeed, got revoked=%v err=%v", revoked, err)
	}
	// Revoking again returns false.
	revokedAgain, err := s.RevokeAPIToken(ctx, userID, tok1.ID)
	if err != nil || revokedAgain {
		t.Fatalf("expected second revoke to return false, got revoked=%v err=%v", revokedAgain, err)
	}

	// Revoked token is rejected on lookup.
	u, lookedUp, err = s.LookupActiveAPITokenUser(ctx, raw1)
	if err != nil || u != nil || lookedUp != nil {
		t.Fatalf("expected revoked token to be rejected, got user=%v tok=%v err=%v", u, lookedUp, err)
	}
}

func TestLookupActiveAPITokenUserRejectsExpiredDeletedAndSuspendedPartner(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Expired token.
	userID := mustCreateUser(t, s, RoleUser)
	expiredRaw := "smk_expired_token"
	if _, err := s.CreateAPIToken(ctx, userID, "expired", "smk_expi", expiredRaw, "agent", sql.NullTime{
		Time:  time.Now().Add(-time.Minute),
		Valid: true,
	}); err != nil {
		t.Fatalf("CreateAPIToken expired: %v", err)
	}
	if u, tok, err := s.LookupActiveAPITokenUser(ctx, expiredRaw); err != nil || u != nil || tok != nil {
		t.Fatalf("expected expired token rejected, got u=%v tok=%v err=%v", u, tok, err)
	}

	// Deleted user.
	deletedUserID := mustCreateUser(t, s, RoleUser)
	deletedRaw := "smk_deleted_user_token"
	if _, err := s.CreateAPIToken(ctx, deletedUserID, "active", "smk_dele", deletedRaw, "agent", sql.NullTime{}); err != nil {
		t.Fatalf("CreateAPIToken deleted user: %v", err)
	}
	if err := s.DeleteUser(ctx, deletedUserID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if u, tok, err := s.LookupActiveAPITokenUser(ctx, deletedRaw); err != nil || u != nil || tok != nil {
		t.Fatalf("expected deleted user token rejected, got u=%v tok=%v err=%v", u, tok, err)
	}

	// Partner user: active partner succeeds, suspended partner fails.
	partnerID, err := s.CreatePartner(ctx, Partner{
		Name:   "Test Partner",
		Slug:   "test-partner-" + nextSuffix(),
		Status: "active",
	})
	if err != nil {
		t.Fatalf("CreatePartner: %v", err)
	}
	suffix := nextSuffix()
	partnerUserID64, err := s.CreatePartnerUser(ctx, User{
		Username:     "partner" + suffix,
		Email:        "partner" + suffix + "@example.com",
		PasswordHash: "hash",
		Role:         RolePartner,
	}, int(partnerID))
	if err != nil {
		t.Fatalf("CreatePartnerUser: %v", err)
	}
	partnerUserID := int(partnerUserID64)
	partnerRaw := "smk_partner_token"
	if _, err := s.CreateAPIToken(ctx, partnerUserID, "partner-agent", "smk_part", partnerRaw, "agent", sql.NullTime{}); err != nil {
		t.Fatalf("CreateAPIToken partner: %v", err)
	}

	if u, tok, err := s.LookupActiveAPITokenUser(ctx, partnerRaw); err != nil || u == nil || tok == nil {
		t.Fatalf("expected active partner token to succeed, got u=%v tok=%v err=%v", u, tok, err)
	}

	if ok, err := s.SetPartnerStatus(ctx, int(partnerID), "suspended"); err != nil || !ok {
		t.Fatalf("SetPartnerStatus: ok=%v err=%v", ok, err)
	}
	if u, tok, err := s.LookupActiveAPITokenUser(ctx, partnerRaw); err != nil || u != nil || tok != nil {
		t.Fatalf("expected suspended partner token rejected, got u=%v tok=%v err=%v", u, tok, err)
	}
}
