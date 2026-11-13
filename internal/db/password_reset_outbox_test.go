package db

import (
	"bytes"
	"testing"
	"time"
)

func TestPasswordResetEmailOutbox_EnqueueLeaseRetryAndSent(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	sealed := []byte("sealed-encrypted-reset-payload")
	expiresAt := time.Now().UTC().Add(30 * time.Minute)

	// Transactional enqueue respects per-user cooldown.
	created, err := s.RequestPasswordResetTokenWithOutbox(t.Context(), userID, "tok-1", expiresAt, sealed)
	chatMust(t, err)
	if !created {
		t.Fatal("expected initial outbox token to be created")
	}
	created2, err := s.RequestPasswordResetTokenWithOutbox(t.Context(), userID, "tok-2", expiresAt, sealed)
	chatMust(t, err)
	if created2 {
		t.Fatal("expected cooldown to block second immediate reset request")
	}

	// Claim leases the entry and blocks a concurrent replica from claiming it.
	claimed, err := s.ClaimPasswordResetEmails(t.Context(), 10, time.Minute)
	chatMust(t, err)
	if len(claimed) != 1 || !bytes.Equal(claimed[0].Payload, sealed) {
		t.Fatalf("claimed = %+v; want 1 matching entry", claimed)
	}
	secondClaim, err := s.ClaimPasswordResetEmails(t.Context(), 10, time.Minute)
	chatMust(t, err)
	if len(secondClaim) != 0 {
		t.Fatalf("second replica claimed %d entries while leased; want 0", len(secondClaim))
	}

	// Renew lease, schedule retry in the past so it becomes claimable again, then mark sent.
	entry := claimed[0]
	chatMust(t, s.RenewPasswordResetEmailLease(t.Context(), entry.ID, entry.Attempts, time.Minute))
	chatMust(t, s.RetryPasswordResetEmail(t.Context(), entry.ID, entry.Attempts, time.Now().UTC().Add(-time.Second)))

	reclaimed, err := s.ClaimPasswordResetEmails(t.Context(), 10, time.Minute)
	chatMust(t, err)
	if len(reclaimed) != 1 || reclaimed[0].ID != entry.ID {
		t.Fatalf("reclaimed = %+v; want entry %d", reclaimed, entry.ID)
	}

	chatMust(t, s.MarkPasswordResetEmailSent(t.Context(), reclaimed[0].ID, reclaimed[0].Attempts))
	afterSent, err := s.ClaimPasswordResetEmails(t.Context(), 10, time.Minute)
	chatMust(t, err)
	if len(afterSent) != 0 {
		t.Fatalf("entries remaining after MarkPasswordResetEmailSent = %d; want 0", len(afterSent))
	}
}
